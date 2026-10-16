// Package realtime mengelola koneksi WebSocket dua arah untuk siswa (/ws) dan admin (/ws/admin).
package realtime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/coder/websocket"
	"golang.org/x/time/rate"
)

const (
	defaultMaxMessageBytes int64         = 8192 // 8 KB
	defaultSendBufferSize  int           = 32
	defaultPingInterval    time.Duration = 25 * time.Second
	defaultWriteTimeout    time.Duration = 5 * time.Second
	defaultRateLimitPerSec float64       = 20
	defaultRateBurst       int           = 30
)

// Envelope adalah pembungkus pesan JSON masuk dari klien: {"event": "<nama>", "data": {...}}.
type Envelope struct {
	Event string          `json:"event"`
	Data  json.RawMessage `json:"data"`
}

// OutEnvelope adalah pembungkus pesan JSON keluar ke klien: {"event": "<nama>", "data": {...}}.
type OutEnvelope struct {
	Event string `json:"event"`
	Data  any    `json:"data"`
}

// Config menampung pengaturan batas dan callback untuk Hub WebSocket.
type Config struct {
	MaxMessageBytes   int64
	SendBufferSize    int
	PingInterval      time.Duration
	WriteTimeout      time.Duration
	RateLimitPerSec   float64
	RateBurst         int
	OnSiswaEvent      func(ctx context.Context, client *Client, env Envelope)
	OnSiswaDisconnect func(ctx context.Context, client *Client)
}

func (c *Config) applyDefaults() {
	if c.MaxMessageBytes <= 0 {
		c.MaxMessageBytes = defaultMaxMessageBytes
	}
	if c.SendBufferSize <= 0 {
		c.SendBufferSize = defaultSendBufferSize
	}
	if c.PingInterval <= 0 {
		c.PingInterval = defaultPingInterval
	}
	if c.WriteTimeout <= 0 {
		c.WriteTimeout = defaultWriteTimeout
	}
	if c.RateLimitPerSec <= 0 {
		c.RateLimitPerSec = defaultRateLimitPerSec
	}
	if c.RateBurst <= 0 {
		c.RateBurst = defaultRateBurst
	}
}

// Client merepresentasikan satu koneksi WebSocket aktif (siswa atau admin).
// Setiap Client memiliki dua goroutine: satu pembaca (readPump) dan satu penulis (writePump).
type Client struct {
	id         string
	peran      string
	siswaID    int
	ujianID    int
	deviceType string
	adminID    int

	hub       *Hub
	conn      *websocket.Conn
	sendCh    chan []byte
	ctx       context.Context
	cancel    context.CancelFunc
	closeOnce sync.Once
}

// ID mengembalikan pengenal unik koneksi soket ini.
func (c *Client) ID() string { return c.id }

// SiswaID mengembalikan ID siswa pemilik koneksi (0 bila admin).
func (c *Client) SiswaID() int { return c.siswaID }

// UjianID mengembalikan ID ujian pada sesi siswa.
func (c *Client) UjianID() int { return c.ujianID }

// DeviceType mengembalikan tipe perangkat ("hp" atau "laptop") dari sesi login siswa.
func (c *Client) DeviceType() string { return c.deviceType }

// Send menyandikan amplop {"event", "data"} ke JSON dan memasukkannya ke antrean kirim klien.
// Jika kanal kirim penuh (klien terlalu lambat membaca), koneksi klien diputus secara asinkron
// dan fungsi ini langsung kembali (false) tanpa memblokir pemanggil (backpressure).
func (c *Client) Send(event string, data any) bool {
	payload, err := json.Marshal(OutEnvelope{
		Event: event,
		Data:  data,
	})
	if err != nil {
		return false
	}
	return c.SendRaw(payload)
}

// SendRaw mengirim byte JSON mentah ke kanal kirim klien secara non-blocking.
func (c *Client) SendRaw(payload []byte) bool {
	select {
	case <-c.ctx.Done():
		return false
	default:
	}

	select {
	case c.sendCh <- payload:
		return true
	default:
		// Kanal penuh: klien lambat membaca pesan, putus koneksinya agar tidak menghambat hub.
		go c.Close(websocket.StatusPolicyViolation, "koneksi terlalu lambat")
		return false
	}
}

// Close menutup koneksi WebSocket secara aman (idempoten lewat sync.Once).
func (c *Client) Close(code websocket.StatusCode, reason string) {
	c.closeOnce.Do(func() {
		c.cancel()
		_ = c.conn.Close(code, reason)
	})
}

// Hub mengelola seluruh koneksi WebSocket aktif dan keanggotaan room pemantauan admin per ujian.
type Hub struct {
	sm  *scs.SessionManager
	cfg Config

	mu         sync.RWMutex
	clients    map[*Client]struct{}
	siswaByKey map[[2]int]*Client
	adminRooms map[int]map[*Client]struct{}

	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	closeOnce sync.Once
}

// NewHub membuat instance Hub WebSocket baru.
func NewHub(sm *scs.SessionManager, cfg Config) *Hub {
	cfg.applyDefaults()
	ctx, cancel := context.WithCancel(context.Background())
	return &Hub{
		sm:         sm,
		cfg:        cfg,
		clients:    make(map[*Client]struct{}),
		siswaByKey: make(map[[2]int]*Client),
		adminRooms: make(map[int]map[*Client]struct{}),
		ctx:        ctx,
		cancel:     cancel,
	}
}

// SetSiswaHandlers mengatur callback event dan disconnect untuk koneksi siswa.
func (h *Hub) SetSiswaHandlers(
	onEvent func(ctx context.Context, client *Client, env Envelope),
	onDisconnect func(ctx context.Context, client *Client),
) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cfg.OnSiswaEvent = onEvent
	h.cfg.OnSiswaDisconnect = onDisconnect
}

// Close menghentikan semua koneksi aktif dan menunggu seluruh goroutine selesai secara tertib.
func (h *Hub) Close() {
	h.closeOnce.Do(func() {
		h.cancel()

		h.mu.RLock()
		active := make([]*Client, 0, len(h.clients))
		for c := range h.clients {
			active = append(active, c)
		}
		h.mu.RUnlock()

		for _, c := range active {
			c.Close(websocket.StatusGoingAway, "server shutdown")
		}

		h.wg.Wait()
	})
}

// HasAdminListeners memeriksa apakah ada minimal satu admin yang sedang memantau room ujianID.
// Digunakan oleh layanan monitor (DESIGN §3.6) agar tidak menjalankan query database bila room kosong.
func (h *Hub) HasAdminListeners(ujianID int) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	room := h.adminRooms[ujianID]
	return len(room) > 0
}

// JoinAdminRoom mendaftarkan koneksi admin ke dalam room pemantauan untuk ujianID tertentu.
func (h *Hub) JoinAdminRoom(c *Client, ujianID int) {
	if ujianID <= 0 {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, exists := h.clients[c]; !exists {
		return
	}
	room, ok := h.adminRooms[ujianID]
	if !ok {
		room = make(map[*Client]struct{})
		h.adminRooms[ujianID] = room
	}
	room[c] = struct{}{}
}

// BroadcastToAdminRoom menyiarkan event ke semua koneksi admin yang tergabung di room ujianID.
func (h *Hub) BroadcastToAdminRoom(ujianID int, event string, data any) {
	h.mu.RLock()
	room := h.adminRooms[ujianID]
	if len(room) == 0 {
		h.mu.RUnlock()
		return
	}
	members := make([]*Client, 0, len(room))
	for c := range room {
		members = append(members, c)
	}
	h.mu.RUnlock()

	payload, err := json.Marshal(OutEnvelope{
		Event: event,
		Data:  data,
	})
	if err != nil {
		return
	}

	for _, c := range members {
		c.SendRaw(payload)
	}
}

// SendToSiswa mengirim event langsung ke koneksi aktif milik siswa pada ujian tertentu (jika sedang terhubung).
func (h *Hub) SendToSiswa(siswaID, ujianID int, event string, data any) bool {
	h.mu.RLock()
	c := h.siswaByKey[[2]int{siswaID, ujianID}]
	h.mu.RUnlock()
	if c == nil {
		return false
	}
	return c.Send(event, data)
}

// ServeSiswaWS adalah HTTP handler untuk endpoint GET /ws (khusus siswa ber-session ujian aktif).
func (h *Hub) ServeSiswaWS() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.sm == nil {
			http.Error(w, `{"error":"Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		peran := h.sm.GetString(r.Context(), "peran")
		siswaID := h.sm.GetInt(r.Context(), "siswaId")
		ujianID := h.sm.GetInt(r.Context(), "ujianId")
		deviceType := h.sm.GetString(r.Context(), "deviceType")

		if peran != "siswa" || siswaID <= 0 || ujianID <= 0 {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"Sesi siswa tidak ditemukan"}`))
			return
		}

		// Pemeriksaan Origin WebSocket tetap aktif (default AcceptOptions = nil)
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}

		c := h.newClient(conn, "siswa", siswaID, ujianID, deviceType, 0)
		h.serveClient(c)
	}
}

// ServeAdminWS adalah HTTP handler untuk endpoint GET /ws/admin (khusus admin ber-session).
func (h *Hub) ServeAdminWS() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.sm == nil {
			http.Error(w, `{"error":"Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		peran := h.sm.GetString(r.Context(), "peran")
		adminID := h.sm.GetInt(r.Context(), "adminId")

		if peran == "" && adminID == 0 {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"Sesi admin tidak ditemukan"}`))
			return
		}
		if peran != "admin" || adminID <= 0 {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"Akses ditolak: hanya admin"}`))
			return
		}

		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}

		c := h.newClient(conn, "admin", 0, 0, "", adminID)
		h.serveClient(c)
	}
}

func (h *Hub) newClient(conn *websocket.Conn, peran string, siswaID, ujianID int, deviceType string, adminID int) *Client {
	conn.SetReadLimit(h.cfg.MaxMessageBytes)
	cCtx, cancel := context.WithCancel(h.ctx)
	return &Client{
		id:         generateSocketID(),
		peran:      peran,
		siswaID:    siswaID,
		ujianID:    ujianID,
		deviceType: deviceType,
		adminID:    adminID,
		hub:        h,
		conn:       conn,
		sendCh:     make(chan []byte, h.cfg.SendBufferSize),
		ctx:        cCtx,
		cancel:     cancel,
	}
}

func (h *Hub) registerClient(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[c] = struct{}{}
	if c.peran == "siswa" && c.siswaID > 0 && c.ujianID > 0 {
		h.siswaByKey[[2]int{c.siswaID, c.ujianID}] = c
	}
}

func (h *Hub) unregisterClient(c *Client) {
	h.mu.Lock()
	delete(h.clients, c)
	if c.peran == "siswa" && c.siswaID > 0 && c.ujianID > 0 {
		key := [2]int{c.siswaID, c.ujianID}
		if h.siswaByKey[key] == c {
			delete(h.siswaByKey, key)
		}
	}
	if c.peran == "admin" {
		for ujianID, room := range h.adminRooms {
			delete(room, c)
			if len(room) == 0 {
				delete(h.adminRooms, ujianID)
			}
		}
	}
	onDisc := h.cfg.OnSiswaDisconnect
	h.mu.Unlock()

	if c.peran == "siswa" && onDisc != nil {
		discCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		onDisc(discCtx, c)
	}
}

func (h *Hub) serveClient(c *Client) {
	select {
	case <-h.ctx.Done():
		c.Close(websocket.StatusGoingAway, "server shutdown")
		return
	default:
	}

	h.registerClient(c)
	h.wg.Add(2)

	go func() {
		defer h.wg.Done()
		c.writePump()
	}()

	go func() {
		defer h.wg.Done()
		defer h.unregisterClient(c)
		c.readPump()
	}()
}

func (c *Client) writePump() {
	ticker := time.NewTicker(c.hub.cfg.PingInterval)
	defer func() {
		ticker.Stop()
		c.Close(websocket.StatusNormalClosure, "")
	}()

	for {
		select {
		case <-c.ctx.Done():
			return
		case msg, ok := <-c.sendCh:
			if !ok {
				return
			}
			wCtx, cancel := context.WithTimeout(c.ctx, c.hub.cfg.WriteTimeout)
			err := c.conn.Write(wCtx, websocket.MessageText, msg)
			cancel()
			if err != nil {
				return
			}
		case <-ticker.C:
			pCtx, cancel := context.WithTimeout(c.ctx, c.hub.cfg.WriteTimeout)
			err := c.conn.Ping(pCtx)
			cancel()
			if err != nil {
				return
			}
		}
	}
}

func (c *Client) readPump() {
	defer c.Close(websocket.StatusNormalClosure, "")

	limiter := rate.NewLimiter(rate.Limit(c.hub.cfg.RateLimitPerSec), c.hub.cfg.RateBurst)

	for {
		_, data, err := c.conn.Read(c.ctx)
		if err != nil {
			return
		}

		if !limiter.Allow() {
			c.Close(websocket.StatusPolicyViolation, "terlalu banyak pesan")
			return
		}

		var env Envelope
		if err := json.Unmarshal(data, &env); err != nil || env.Event == "" {
			continue
		}

		if c.peran == "admin" {
			c.handleAdminEvent(env)
			continue
		}

		c.hub.mu.RLock()
		onEvent := c.hub.cfg.OnSiswaEvent
		c.hub.mu.RUnlock()

		if onEvent != nil {
			onEvent(c.ctx, c, env)
		}
	}
}

func (c *Client) handleAdminEvent(env Envelope) {
	if env.Event != "admin:join" {
		return
	}

	var payload struct {
		UjianID any `json:"ujian_id"`
	}
	if len(env.Data) > 0 {
		_ = json.Unmarshal(env.Data, &payload)
	}

	var ujianID int
	switch v := payload.UjianID.(type) {
	case float64:
		ujianID = int(v)
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			ujianID = parsed
		}
	}

	if ujianID <= 0 {
		c.Send("admin:error", map[string]any{
			"message": "ujian_id tidak valid",
			"code":    400,
		})
		return
	}

	c.hub.JoinAdminRoom(c, ujianID)
	c.Send("admin:joined", map[string]any{
		"ujian_id": ujianID,
		"room":     fmt.Sprintf("admin:%d", ujianID),
	})
}

func generateSocketID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(b[:])
}
