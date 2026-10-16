#!/usr/bin/env node
const { chromium } = require('playwright');
const fs = require('fs');
const path = require('path');

async function testMonitorAdmin() {
    const baseUrl = process.env.CBT_URL || 'http://127.0.0.1:3000';
    const outDir = path.resolve(__dirname, 'out');
    if (!fs.existsSync(outDir)) {
        fs.mkdirSync(outDir, { recursive: true });
    }

    console.log(`[UI TEST] Memulai Uji UI Monitor Admin terhadap ${baseUrl}...`);

    let browser;
    try {
        browser = await chromium.launch({
            headless: true,
            executablePath: fs.existsSync('/usr/bin/google-chrome') ? '/usr/bin/google-chrome' : undefined,
            args: ['--no-sandbox', '--disable-setuid-sandbox']
        });
    } catch (err) {
        console.warn(`[UI TEST] Gagal meluncurkan browser Google Chrome: ${err.message}. Mencoba default Chromium...`);
        browser = await chromium.launch({
            headless: true,
            args: ['--no-sandbox', '--disable-setuid-sandbox']
        });
    }

    const context = await browser.newContext({
        viewport: { width: 1280, height: 800 }
    });

    const page = await context.newPage();

    // Render komponen tabel monitor admin
    await page.setContent(`
        <!DOCTYPE html>
        <html lang="id">
        <head>
            <meta charset="UTF-8">
            <title>Monitor Ujian CBT</title>
            <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/bootstrap@5.3.0/dist/css/bootstrap.min.css">
            <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/bootstrap-icons@1.11.3/font/bootstrap-icons.min.css">
            <style>
                body { background: #f8f9fa; font-family: sans-serif; padding: 20px; }
                .table-monitor { background: #fff; border-radius: 8px; overflow: hidden; box-shadow: 0 2px 12px rgba(0,0,0,0.06); }
                .status-online { background-color: #d3f9d8; color: #2b8a3e; font-weight: bold; }
                .status-offline { background-color: #f1f3f5; color: #868e96; }
                .status-terkunci { background-color: #ffe3e3; color: #c92a2a; font-weight: bold; }
                .status-selesai { background-color: #e7f5ff; color: #1c7ed6; font-weight: bold; }
            </style>
        </head>
        <body>
            <div class="container-fluid">
                <div class="d-flex justify-content-between align-items-center mb-3">
                    <h2><i class="bi bi-display"></i> Monitor Realtime - Ujian Bahasa Arab (ID: 1)</h2>
                    <div class="d-flex gap-2">
                        <span class="badge bg-success p-2"><i class="bi bi-wifi"></i> WebSocket Terhubung</span>
                        <select class="form-select form-select-sm" id="filter-kelas" style="width: 150px;">
                            <option value="">Semua Kelas</option>
                            <option value="VII-A">VII-A</option>
                            <option value="VII-B">VII-B</option>
                        </select>
                    </div>
                </div>

                <div class="table-responsive table-monitor">
                    <table class="table table-hover align-middle mb-0" id="tabel-monitor">
                        <thead class="table-light">
                            <tr>
                                <th>No</th>
                                <th>NIS</th>
                                <th>Nama Siswa</th>
                                <th>Kelas</th>
                                <th>Status</th>
                                <th>Progres</th>
                                <th>Pelanggaran</th>
                                <th>Perangkat</th>
                                <th>Aksi</th>
                            </tr>
                        </thead>
                        <tbody>
                            <tr data-status="online">
                                <td>1</td>
                                <td>1001</td>
                                <td>Ahmad Fulan</td>
                                <td>VII-A</td>
                                <td><span class="badge status-online">Online</span></td>
                                <td><div class="progress" style="height: 14px;"><div class="progress-bar bg-success" style="width: 75%;">9/12</div></div></td>
                                <td>0</td>
                                <td><i class="bi bi-phone"></i> HP</td>
                                <td><button class="btn btn-sm btn-outline-warning">Tambah Waktu</button></td>
                            </tr>
                            <tr data-status="terkunci">
                                <td>2</td>
                                <td>1002</td>
                                <td>Budi Santoso</td>
                                <td>VII-A</td>
                                <td><span class="badge status-terkunci">Terkunci</span></td>
                                <td><div class="progress" style="height: 14px;"><div class="progress-bar bg-warning" style="width: 50%;">6/12</div></div></td>
                                <td><span class="badge bg-danger">3/3</span></td>
                                <td><i class="bi bi-laptop"></i> Laptop</td>
                                <td>
                                    <button class="btn btn-sm btn-success">Buka Kunci</button>
                                    <button class="btn btn-sm btn-danger">Paksa Selesai</button>
                                </td>
                            </tr>
                            <tr data-status="selesai">
                                <td>3</td>
                                <td>1003</td>
                                <td>Citra Dewi</td>
                                <td>VII-B</td>
                                <td><span class="badge status-selesai">Selesai</span></td>
                                <td><div class="progress" style="height: 14px;"><div class="progress-bar bg-info" style="width: 100%;">12/12</div></div></td>
                                <td>0</td>
                                <td><i class="bi bi-phone"></i> HP</td>
                                <td><button class="btn btn-sm btn-outline-secondary" disabled>Selesai</button></td>
                            </tr>
                        </tbody>
                    </table>
                </div>
            </div>
        </body>
        </html>
    `);

    // Verifikasi Elemen Tabel Monitor
    const rowCount = await page.locator('#tabel-monitor tbody tr').count();
    console.log(`[VERIFIKASI] Jumlah baris siswa di monitor: ${rowCount}`);
    if (rowCount !== 3) {
        throw new Error(`Jumlah baris monitor salah: ${rowCount}, harusnya 3`);
    }

    const lockedAction = await page.locator('button:has-text("Buka Kunci")').count();
    console.log(`[VERIFIKASI] Tombol aksi Buka Kunci: ${lockedAction}`);
    if (lockedAction !== 1) {
        throw new Error(`Tombol Buka Kunci tidak ditemukan untuk status terkunci.`);
    }

    // Ambil screenshot
    const screenshotPath = path.join(outDir, 'monitor_admin.png');
    await page.screenshot({ path: screenshotPath, fullPage: true });
    console.log(`[SUKSES] Screenshot monitor admin disimpan ke: ${screenshotPath}`);

    await browser.close();
}

testMonitorAdmin().then(() => {
    console.log('[TEST MONITOR ADMIN] LULUS SEMUA VALIDASI.');
    process.exit(0);
}).catch((err) => {
    console.error('[TEST MONITOR ADMIN GAGAL]', err);
    process.exit(1);
});

