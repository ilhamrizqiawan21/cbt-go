#!/usr/bin/env node
const http = require('http');
const https = require('https');
const mysql = require('mysql2/promise');
const bcrypt = require('bcrypt');
const path = require('path');
require('dotenv').config();

// HTTP Agent dengan keep-alive dan connection pooling untuk 410 klien
const httpAgent = new http.Agent({
    keepAlive: true,
    maxSockets: 600,
    maxFreeSockets: 100,
    timeout: 30000
});

/**
 * Helper request HTTP mandiri yang mengukur latensi dan menangani cookie sesi
 */
function sendRequest({ method, url, body, cookie, agent = httpAgent }) {
    return new Promise((resolve) => {
        const u = new URL(url);
        const startTime = Date.now();
        const headers = {
            'User-Agent': 'CBT-LoadTest/1.0',
            'Connection': 'keep-alive'
        };

        let payload = null;
        if (body) {
            if (typeof body === 'object') {
                payload = JSON.stringify(body);
                headers['Content-Type'] = 'application/json';
            } else {
                payload = String(body);
                headers['Content-Type'] = 'application/x-www-form-urlencoded';
            }
            headers['Content-Length'] = Buffer.byteLength(payload);
        }

        if (cookie) {
            headers['Cookie'] = cookie;
        }

        const req = http.request({
            protocol: u.protocol,
            hostname: u.hostname,
            port: u.port || (u.protocol === 'https:' ? 443 : 80),
            path: u.pathname + u.search,
            method,
            headers,
            agent
        }, (res) => {
            let data = '';
            res.on('data', chunk => { data += chunk; });
            res.on('end', () => {
                const duration = Date.now() - startTime;
                let newCookie = cookie;
                const setCookie = res.headers['set-cookie'];
                if (setCookie && setCookie.length > 0) {
                    newCookie = setCookie.map(c => c.split(';')[0]).join('; ');
                }

                resolve({
                    statusCode: res.statusCode,
                    duration,
                    cookie: newCookie,
                    headers: res.headers,
                    body: data,
                    error: null
                });
            });
        });

        req.on('error', (err) => {
            const duration = Date.now() - startTime;
            resolve({
                statusCode: 0,
                duration,
                cookie,
                headers: {},
                body: '',
                error: err.message
            });
        });

        if (payload) {
            req.write(payload);
        }
        req.end();
    });
}

/**
 * Seeding data sintetis khusus untuk load test
 */
async function seedLoadTestData(pool, count = 410) {
    console.log(`[SEED LOADTEST] Menyiapkan data untuk ${count} peserta...`);

    // 1. Kelas khusus load test
    await pool.query("INSERT IGNORE INTO kelas (nama_kelas) VALUES ('LOAD-TEST')");
    const [kelasRows] = await pool.query("SELECT id FROM kelas WHERE nama_kelas = 'LOAD-TEST' LIMIT 1");
    const kelasId = kelasRows[0].id;

    // 2. Guru & Mapel
    await pool.query("INSERT IGNORE INTO mata_pelajaran (nama_mapel) VALUES ('Uji Beban Komputasi')");
    const [mapelRows] = await pool.query("SELECT id FROM mata_pelajaran WHERE nama_mapel = 'Uji Beban Komputasi' LIMIT 1");
    const mapelId = mapelRows[0].id;

    const hashPin = await bcrypt.hash('1234', 10);
    const [guruExist] = await pool.query("SELECT id FROM guru WHERE username = 'guru_loadtest' LIMIT 1");
    let guruId;
    if (guruExist.length === 0) {
        const [resG] = await pool.query(
            "INSERT INTO guru (nip, nama, username, password) VALUES ('1990010199', 'Guru Loadtest', 'guru_loadtest', ?)",
            [hashPin]
        );
        guruId = resG.insertId;
    } else {
        guruId = guruExist[0].id;
    }

    // 3. Pengajaran
    await pool.query(
        "INSERT IGNORE INTO pengajaran (guru_id, mapel_id, kelas_id) VALUES (?, ?, ?)",
        [guruId, mapelId, kelasId]
    );
    const [pengajaranRows] = await pool.query(
        "SELECT id FROM pengajaran WHERE guru_id = ? AND mapel_id = ? AND kelas_id = ? LIMIT 1",
        [guruId, mapelId, kelasId]
    );
    const pengajaranId = pengajaranRows[0].id;

    // 4. Ujian
    const [ujianExist] = await pool.query(
        "SELECT id FROM ujian WHERE pengajaran_id = ? AND nama_ujian = 'Simulasi Beban 410 Siswa' LIMIT 1",
        [pengajaranId]
    );
    let ujianId;
    if (ujianExist.length === 0) {
        const [resU] = await pool.query(
            `INSERT INTO ujian (pengajaran_id, nama_ujian, durasi, tanggal_mulai, tanggal_selesai, acak_soal, acak_pilihan, batas_pelanggaran)
             VALUES (?, 'Simulasi Beban 410 Siswa', 120, NOW() - INTERVAL 10 MINUTE, NOW() + INTERVAL 2 HOUR, 1, 1, 5)`,
            [pengajaranId]
        );
        ujianId = resU.insertId;

        // Buat 10 butir soal PG + 1 essay
        for (let i = 1; i <= 10; i++) {
            await pool.query(
                `INSERT INTO soal (ujian_id, tipe_soal, teks_soal, poin, pilihan_a, pilihan_b, pilihan_c, pilihan_d, jawaban_benar)
                 VALUES (?, 'pg', ?, 2, 'Pilihan A', 'Pilihan B', 'Pilihan C', 'Pilihan D', 'A')`,
                [ujianId, `Pertanyaan Uji Beban Nomor ${i}: Berapakah nilai efisiensi sistem?`]
            );
        }
        await pool.query(
            `INSERT INTO soal (ujian_id, tipe_soal, teks_soal, poin)
             VALUES (?, 'essay', 'Tuliskan kesimpulan evaluasi sistem pada lembar jawaban kertas.', 4)`,
            [ujianId]
        );
    } else {
        ujianId = ujianExist[0].id;
    }

    // 5. Seed siswa sintetis secara batch
    const [existingSiswa] = await pool.query(
        "SELECT nis FROM siswa WHERE kelas = 'LOAD-TEST'"
    );
    const existingNisSet = new Set(existingSiswa.map(s => s.nis));

    const siswaToInsert = [];
    for (let i = 1; i <= count; i++) {
        const nis = `LOAD_${String(i).padStart(4, '0')}`;
        if (!existingNisSet.has(nis)) {
            siswaToInsert.push([nis, `Peserta Beban ${i}`, 'LOAD-TEST', hashPin]);
        }
    }

    if (siswaToInsert.length > 0) {
        console.log(`[SEED LOADTEST] Memasukkan ${siswaToInsert.length} data siswa baru...`);
        const batchSize = 100;
        for (let b = 0; b < siswaToInsert.length; b += batchSize) {
            const batch = siswaToInsert.slice(b, b + batchSize);
            await pool.query(
                "INSERT INTO siswa (nis, nama, kelas, pin_ujian) VALUES ?",
                [batch]
            );
        }
    }

    const [allStudents] = await pool.query(
        "SELECT id, nis FROM siswa WHERE kelas = 'LOAD-TEST' ORDER BY nis ASC LIMIT ?",
        [count]
    );

    const [soalRows] = await pool.query(
        "SELECT id, tipe_soal FROM soal WHERE ujian_id = ? ORDER BY id ASC",
        [ujianId]
    );

    return {
        kelasId,
        ujianId,
        soalList: soalRows,
        students: allStudents
    };
}

/**
 * Membersihkan data sintetis load test
 */
async function cleanupLoadTestData(pool) {
    console.log('[CLEANUP LOADTEST] Membersihkan data sintetis...');
    await pool.query("SET FOREIGN_KEY_CHECKS = 0");
    await pool.query("DELETE FROM siswa WHERE kelas = 'LOAD-TEST'");
    await pool.query("DELETE FROM kelas WHERE nama_kelas = 'LOAD-TEST'");
    await pool.query("DELETE FROM mata_pelajaran WHERE nama_mapel = 'Uji Beban Komputasi'");
    await pool.query("DELETE FROM guru WHERE username = 'guru_loadtest'");
    await pool.query("SET FOREIGN_KEY_CHECKS = 1");
}

/**
 * Kalkulasi persentil
 */
function calculatePercentile(numbers, p) {
    if (numbers.length === 0) return 0;
    const sorted = [...numbers].sort((a, b) => a - b);
    const index = Math.ceil((p / 100) * sorted.length) - 1;
    return sorted[Math.max(0, Math.min(index, sorted.length - 1))];
}

/**
 * Eksekutor simulasi load test
 */
async function runLoadTest(options = {}) {
    const baseUrl = options.url || 'http://127.0.0.1:3000';
    const clientCount = options.clients || 410;
    const rampUpMs = options.rampUpMs !== undefined ? options.rampUpMs : 300000; // default 5 menit (300 dtk)
    const durationMs = options.durationMs !== undefined ? options.durationMs : 300000;
    const disconnectRatio = options.disconnectRatio !== undefined ? options.disconnectRatio : 0.15; // 15% putus-sambung
    const pool = options.pool;

    if (!pool) {
        throw new Error('Pool database diperlukan untuk menjalankan uji beban dan audit data');
    }

    const testData = await seedLoadTestData(pool, clientCount);
    const { ujianId, soalList, students } = testData;

    console.log(`\n============================================================`);
    console.log(`[LOADTEST] Memulai Simulasi Uji Beban:`);
    console.log(`- Target URL       : ${baseUrl}`);
    console.log(`- Jumlah Peserta   : ${students.length}`);
    console.log(`- Ujian ID         : ${ujianId} (${soalList.length} soal)`);
    console.log(`- Durasi Ramp-up   : ${(rampUpMs / 1000).toFixed(1)} detik`);
    console.log(`- Target Durasi    : ${(durationMs / 1000).toFixed(1)} detik`);
    console.log(`- Rasio Putus      : ${(disconnectRatio * 100).toFixed(0)}%`);
    console.log(`============================================================\n`);

    const stats = {
        totalRequests: 0,
        totalSuccess: 0,
        totalFailed: 0,
        totalAnswersSent: 0,
        endpoints: {
            'POST /login-siswa': { count: 0, success: 0, failed: 0, latencies: [] },
            'GET /api/soal/:ujianId': { count: 0, success: 0, failed: 0, latencies: [] },
            'GET /api/sesi': { count: 0, success: 0, failed: 0, latencies: [] },
            'POST /api/sinkron-jawaban': { count: 0, success: 0, failed: 0, latencies: [] },
            'POST /api/heartbeat': { count: 0, success: 0, failed: 0, latencies: [] },
            'POST /api/selesai-ujian': { count: 0, success: 0, failed: 0, latencies: [] }
        }
    };

    function recordMetric(endpoint, res) {
        stats.totalRequests++;
        const ep = stats.endpoints[endpoint];
        if (ep) {
            ep.count++;
            ep.latencies.push(res.duration);
            if (res.statusCode >= 200 && res.statusCode < 400) {
                ep.success++;
                stats.totalSuccess++;
            } else {
                ep.failed++;
                stats.totalFailed++;
            }
        }
    }

    const startTime = Date.now();
    const memoryInitial = process.memoryUsage();

    // Fungsi simulasi 1 siswa
    async function simulateStudent(student, index) {
        // Ramp-up delay merata dalam rentang rampUpMs
        const delay = rampUpMs > 0 ? (index / students.length) * rampUpMs : 0;
        if (delay > 0) {
            await new Promise(r => setTimeout(r, delay));
        }

        let cookie = null;

        // 1. Login siswa
        const loginPayload = `nis=${student.nis}&pin=1234&ujian_id=${ujianId}&device_type=${index % 2 === 0 ? 'laptop' : 'hp'}`;
        const loginRes = await sendRequest({
            method: 'POST',
            url: `${baseUrl}/login-siswa`,
            body: loginPayload
        });
        recordMetric('POST /login-siswa', loginRes);

        if (loginRes.statusCode !== 302 && loginRes.statusCode !== 200) {
            return;
        }
        cookie = loginRes.cookie;

        // 2. Unduh paket soal & sesi
        const soalRes = await sendRequest({
            method: 'GET',
            url: `${baseUrl}/api/soal/${ujianId}`,
            cookie
        });
        recordMetric('GET /api/soal/:ujianId', soalRes);

        const sesiRes = await sendRequest({
            method: 'GET',
            url: `${baseUrl}/api/sesi`,
            cookie
        });
        recordMetric('GET /api/sesi', sesiRes);

        // 3. Simulasi Menjawab Soal & Sinkronisasi Berkala
        const pgSoals = soalList.filter(s => s.tipe_soal === 'pg');
        const answerBatches = [
            pgSoals.slice(0, 3),
            pgSoals.slice(3, 7),
            pgSoals.slice(7, 10)
        ];

        const optionsArr = ['A', 'B', 'C', 'D'];
        let isDisconnected = (index % Math.ceil(1 / disconnectRatio) === 0);

        for (let bIdx = 0; bIdx < answerBatches.length; bIdx++) {
            const batchSoals = answerBatches[bIdx];
            if (batchSoals.length === 0) continue;

            // Heartbeat sebelum sinkron
            const hbRes = await sendRequest({
                method: 'POST',
                url: `${baseUrl}/api/heartbeat`,
                cookie
            });
            recordMetric('POST /api/heartbeat', hbRes);

            // Simulasi putus-sambung jika peserta terpilih
            if (isDisconnected && bIdx === 1) {
                // Jeda offline 2 detik lalu re-login (take-over)
                await new Promise(r => setTimeout(r, 2000));
                const reLoginRes = await sendRequest({
                    method: 'POST',
                    url: `${baseUrl}/login-siswa`,
                    body: loginPayload
                });
                recordMetric('POST /login-siswa', reLoginRes);
                if (reLoginRes.cookie) {
                    cookie = reLoginRes.cookie;
                }
            }

            const items = batchSoals.map((s, idx) => ({
                soal_id: s.id,
                jawaban_dipilih: optionsArr[(index + idx) % optionsArr.length],
                client_ts: new Date().toISOString().slice(0, 19).replace('T', ' ')
            }));

            const syncRes = await sendRequest({
                method: 'POST',
                url: `${baseUrl}/api/sinkron-jawaban`,
                body: { items },
                cookie
            });
            recordMetric('POST /api/sinkron-jawaban', syncRes);

            if (syncRes.statusCode === 200) {
                stats.totalAnswersSent += items.length;
            }

            // Jeda antar batch
            await new Promise(r => setTimeout(r, 500));
        }

        // 4. Selesai Ujian
        const finishRes = await sendRequest({
            method: 'POST',
            url: `${baseUrl}/api/selesai-ujian`,
            cookie
        });
        recordMetric('POST /api/selesai-ujian', finishRes);
    }

    // Jalankan semua siswa secara paralel terkelola
    const promises = students.map((s, idx) => simulateStudent(s, idx));
    await Promise.all(promises);

    const totalDurationSec = (Date.now() - startTime) / 1000;
    const memoryFinal = process.memoryUsage();

    // 5. Audit Integritas Data di Database
    const [dbAnswerCountRows] = await pool.query(
        `SELECT COUNT(*) AS total_db_answers
         FROM jawaban_siswa js
         JOIN siswa s ON js.siswa_id = s.id
         WHERE js.ujian_id = ? AND s.kelas = 'LOAD-TEST'`,
        [ujianId]
    );
    const answersInDb = Number(dbAnswerCountRows[0].total_db_answers || 0);
    const answersLost = Math.max(0, stats.totalAnswersSent - answersInDb);

    // Hitung statistik per endpoint
    const summary = {};
    for (const [name, ep] of Object.entries(stats.endpoints)) {
        const lats = ep.latencies;
        const sum = lats.reduce((a, b) => a + b, 0);
        summary[name] = {
            count: ep.count,
            success: ep.success,
            failed: ep.failed,
            min: lats.length ? Math.min(...lats) : 0,
            avg: lats.length ? (sum / lats.length).toFixed(1) : 0,
            p50: calculatePercentile(lats, 50),
            p90: calculatePercentile(lats, 90),
            p95: calculatePercentile(lats, 95),
            max: lats.length ? Math.max(...lats) : 0
        };
    }

    const syncP95 = summary['POST /api/sinkron-jawaban'] ? summary['POST /api/sinkron-jawaban'].p95 : 0;
    const errorRatePct = stats.totalRequests > 0 ? (stats.totalFailed / stats.totalRequests) * 100 : 0;

    const report = {
        totalStudents: students.length,
        durationSeconds: totalDurationSec.toFixed(2),
        totalRequests: stats.totalRequests,
        totalSuccess: stats.totalSuccess,
        totalFailed: stats.totalFailed,
        errorRatePercent: errorRatePct.toFixed(2),
        totalAnswersSent: stats.totalAnswersSent,
        answersInDatabase: answersInDb,
        answersLost,
        syncP95Ms: syncP95,
        passThresholds: {
            syncP95Under500ms: syncP95 < 500,
            errorRateUnder1Pct: errorRatePct < 1.0,
            zeroAnswersLost: answersLost === 0
        },
        memory: {
            initialRssMb: (memoryInitial.rss / 1024 / 1024).toFixed(1),
            finalRssMb: (memoryFinal.rss / 1024 / 1024).toFixed(1)
        },
        endpoints: summary
    };

    console.log(`\n================== HASIL UJI BEBAN ==================`);
    console.log(`Waktu Total        : ${report.durationSeconds} detik`);
    console.log(`Total Permintaan   : ${report.totalRequests}`);
    console.log(`Permintaan Berhasil: ${report.totalSuccess}`);
    console.log(`Permintaan Gagal   : ${report.totalFailed} (${report.errorRatePercent}%)`);
    console.log(`Jawaban Dikirim    : ${report.totalAnswersSent}`);
    console.log(`Jawaban di DB      : ${report.answersInDatabase}`);
    console.log(`Jawaban Hilang     : ${report.answersLost}`);
    console.log(`Memory RSS         : ${report.memory.initialRssMb} MB -> ${report.memory.finalRssMb} MB`);
    console.log(`-----------------------------------------------------`);
    console.log(`Rincian Respons per Endpoint:`);
    for (const [epName, epStats] of Object.entries(summary)) {
        console.log(`  ${epName.padEnd(28)} | Req: ${String(epStats.count).padStart(4)} | Sukses: ${String(epStats.success).padStart(4)} | Min: ${String(epStats.min).padStart(4)}ms | Avg: ${String(epStats.avg).padStart(6)}ms | p95: ${String(epStats.p95).padStart(4)}ms | Max: ${String(epStats.max).padStart(5)}ms`);
    }
    console.log(`-----------------------------------------------------`);
    console.log(`AMBANG TERIMA:`);
    console.log(`  1. p95 sinkron-jawaban < 500ms: ${syncP95}ms [${report.passThresholds.syncP95Under500ms ? 'LULUS' : 'GAGAL'}]`);
    console.log(`  2. Error Rate < 1%             : ${report.errorRatePercent}% [${report.passThresholds.errorRateUnder1Pct ? 'LULUS' : 'GAGAL'}]`);
    console.log(`  3. Tanpa Jawaban Hilang        : ${answersLost} hilang [${report.passThresholds.zeroAnswersLost ? 'LULUS' : 'GAGAL'}]`);
    console.log(`=====================================================\n`);

    return report;
}

if (require.main === module) {
    const args = process.argv.slice(2);
    let clients = 410;
    let rampUp = 300; // detik
    let url = process.env.LOADTEST_URL || 'http://127.0.0.1:3000';
    let cleanupAfter = false;

    for (const arg of args) {
        if (arg.startsWith('--clients=')) clients = Number(arg.split('=')[1]);
        if (arg.startsWith('--ramp-up=')) rampUp = Number(arg.split('=')[1]);
        if (arg.startsWith('--url=')) url = arg.split('=')[1];
        if (arg === '--cleanup') cleanupAfter = true;
    }

    const pool = require('../models/db');

    runLoadTest({
        url,
        clients,
        rampUpMs: rampUp * 1000,
        pool
    }).then(async (report) => {
        if (cleanupAfter) {
            await cleanupLoadTestData(pool);
        }
        await pool.end();
        const allPassed = Object.values(report.passThresholds).every(Boolean);
        process.exit(allPassed ? 0 : 1);
    }).catch(async (err) => {
        console.error('[LOADTEST ERROR]', err);
        try { await pool.end(); } catch (_) {}
        process.exit(1);
    });
}

module.exports = {
    runLoadTest,
    seedLoadTestData,
    cleanupLoadTestData,
    sendRequest
};
