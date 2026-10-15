// Package db membuka dan mengatur pool koneksi MySQL/MariaDB.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/go-sql-driver/mysql"

	"cbt-go/internal/config"
)

const (
	pingTimeout     = 5 * time.Second
	connMaxLifetime = 5 * time.Minute
)

// Open membuka pool koneksi dan memastikan database bisa dihubungi.
//
// Pool (*sql.DB) aman dipakai bersamaan oleh banyak goroutine, jadi cukup
// dibuat sekali di main lalu dibagikan ke package lain.
func Open(ctx context.Context, c config.DB, loc *time.Location) (*sql.DB, error) {
	mc := mysql.NewConfig()
	mc.User = c.User
	mc.Passwd = c.Password
	mc.DBName = c.Name
	mc.ParseTime = true // DATETIME -> time.Time
	mc.Loc = loc
	mc.Collation = "utf8mb4_unicode_ci"
	if c.Socket != "" {
		mc.Net = "unix"
		mc.Addr = c.Socket
	} else {
		mc.Net = "tcp"
		mc.Addr = net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
	}

	connector, err := mysql.NewConnector(mc)
	if err != nil {
		return nil, fmt.Errorf("db: konfigurasi koneksi tidak valid: %w", err)
	}

	pool := sql.OpenDB(connector)
	pool.SetMaxOpenConns(c.MaxOpenConns)
	pool.SetMaxIdleConns(c.MaxOpenConns)
	pool.SetConnMaxLifetime(connMaxLifetime)

	pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()
	if err := pool.PingContext(pingCtx); err != nil {
		_ = pool.Close()
		return nil, fmt.Errorf("db: tidak bisa terhubung ke %s: %w", mc.Addr, err)
	}
	return pool, nil
}
