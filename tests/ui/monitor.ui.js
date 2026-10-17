#!/usr/bin/env node
// Uji UI halaman monitor admin terhadap SERVER GO SUNGGUHAN (bukan HTML statis).
// Prasyarat: server berjalan (CBT_URL) dengan data seed (`cmd/seed`).
// Env: CBT_URL (default http://127.0.0.1:3000), CBT_ADMIN_USER (admin), CBT_ADMIN_PASS (admin123).
const { chromium } = require('playwright');
const fs = require('fs');
const path = require('path');

async function main() {
    const baseUrl = process.env.CBT_URL || 'http://127.0.0.1:3000';
    const user = process.env.CBT_ADMIN_USER || 'admin';
    const pass = process.env.CBT_ADMIN_PASS || 'admin123';
    const outDir = path.resolve(__dirname, 'out');
    fs.mkdirSync(outDir, { recursive: true });

    // Pakai Chrome sistem bila ada (browser bawaan Playwright tidak wajib diunduh).
    const chrome = ['/usr/bin/google-chrome', '/usr/bin/chromium', '/usr/bin/chromium-browser'].find((p) => fs.existsSync(p));
    const browser = await chromium.launch({ headless: true, args: ['--no-sandbox'], executablePath: chrome });
    try {
        const page = await (await browser.newContext({ viewport: { width: 1280, height: 800 } })).newPage();
        const errors = [];
        page.on('pageerror', (e) => errors.push(e.message));

        await page.goto(`${baseUrl}/login-admin`, { waitUntil: 'domcontentloaded' });
        await page.fill('input[name="username"]', user);
        await page.fill('input[name="password"]', pass);
        await Promise.all([page.waitForURL(/\/admin\//), page.click('button[type="submit"]')]);

        await page.goto(`${baseUrl}/admin/monitor`, { waitUntil: 'domcontentloaded' });
        // WebSocket admin bergabung ke room ujian → badge "Realtime aktif".
        await page.waitForFunction(() => /Realtime aktif/.test(document.getElementById('ws-badge')?.textContent || ''), null, { timeout: 10000 });
        await page.waitForSelector('#tabel-body tr[data-status]', { timeout: 10000 });

        const hasil = await page.evaluate(() => {
            const baris = [...document.querySelectorAll('#tabel-body tr[data-status]')];
            const total = parseInt(document.getElementById('stat-total_siswa').textContent, 10);
            return { baris: baris.length, total, status: [...new Set(baris.map((b) => b.dataset.status))] };
        });
        console.log('[VERIFIKASI]', JSON.stringify(hasil));
        if (hasil.baris === 0) throw new Error('Tabel monitor kosong');
        if (hasil.baris !== hasil.total) throw new Error(`Jumlah baris (${hasil.baris}) ≠ kartu total (${hasil.total})`);

        // Filter status bekerja di sisi klien.
        const pertama = hasil.status[0];
        await page.selectOption('#filter-status', pertama);
        const terlihat = await page.$$eval('#tabel-body tr[data-status]', (rs) => rs.filter((r) => r.style.display !== 'none').map((r) => r.dataset.status));
        if (!terlihat.length || terlihat.some((s) => s !== pertama)) throw new Error('Filter status tidak bekerja');

        if (errors.length) throw new Error('Error JavaScript di halaman: ' + errors.join('; '));
        await page.screenshot({ path: path.join(outDir, 'monitor_admin.png'), fullPage: true });
        console.log('[TEST MONITOR ADMIN] LULUS');
    } finally {
        await browser.close();
    }
}

main().catch((err) => {
    console.error('[TEST MONITOR ADMIN GAGAL]', err.message || err);
    process.exit(1);
});
