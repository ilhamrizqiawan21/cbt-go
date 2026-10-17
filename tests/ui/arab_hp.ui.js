#!/usr/bin/env node
// Uji UI halaman ujian siswa di HP 360×640 terhadap SERVER GO SUNGGUHAN (bukan HTML statis).
// Prasyarat: server berjalan (CBT_URL) dengan data seed (`cmd/seed`), sebaiknya DB uji.
// Env: CBT_URL (default http://127.0.0.1:3000), CBT_NIS (1002), CBT_PIN (1234),
//      CBT_UJIAN (teks opsi ujian, default "Ujian Bahasa Arab").
// Catatan: login ini membuat/mengambil alih sesi ujian siswa tersebut di database server.
const { chromium, devices } = require('playwright');
const fs = require('fs');
const path = require('path');

async function main() {
    const baseUrl = process.env.CBT_URL || 'http://127.0.0.1:3000';
    const nis = process.env.CBT_NIS || '1002';
    const pin = process.env.CBT_PIN || '1234';
    const namaUjian = process.env.CBT_UJIAN || 'Ujian Bahasa Arab';
    const outDir = path.resolve(__dirname, 'out');
    fs.mkdirSync(outDir, { recursive: true });

    // Pakai Chrome sistem bila ada (browser bawaan Playwright tidak wajib diunduh).
    const chrome = ['/usr/bin/google-chrome', '/usr/bin/chromium', '/usr/bin/chromium-browser'].find((p) => fs.existsSync(p));
    const browser = await chromium.launch({ headless: true, args: ['--no-sandbox'], executablePath: chrome });
    try {
        const context = await browser.newContext({ ...devices['Galaxy S9+'], viewport: { width: 360, height: 640 } });
        const page = await context.newPage();
        const errors = [];
        page.on('pageerror', (e) => errors.push(e.message));

        await page.goto(`${baseUrl}/login`, { waitUntil: 'domcontentloaded' });
        await page.fill('input[name="nis"]', nis);
        await page.fill('input[name="pin"]', pin);
        await page.waitForFunction(() => document.querySelectorAll('#ujian_id option').length > 1);
        const nilaiOpsi = await page.$$eval('#ujian_id option', (opsi, nama) => {
            const o = opsi.find((x) => x.textContent.includes(nama));
            return o ? o.value : null;
        }, namaUjian);
        if (!nilaiOpsi) {
            const daftar = await page.$$eval('#ujian_id option', (o) => o.map((x) => x.textContent.trim()));
            throw new Error(`Ujian "${namaUjian}" tidak ada di daftar: ${daftar.join(' | ')}`);
        }
        await page.selectOption('#ujian_id', nilaiOpsi);
        await Promise.all([page.waitForURL(/\/ujian/, { timeout: 10000 }), page.click('form[action="/login-siswa"] button[type="submit"]')]);

        // Soal dimuat dari /api/soal setelah WebSocket mengirim mulai-ujian.
        await page.waitForSelector('#soal-container .soal-text', { timeout: 15000 });

        const hasil = await page.evaluate(() => {
            const teks = [...document.querySelectorAll('.soal-text')];
            const arab = teks.filter((el) => /[؀-ۿ]/.test(el.textContent));
            return {
                jumlahSoal: teks.length,
                soalArab: arab.length,
                semuaDirAuto: teks.every((el) => el.getAttribute('dir') === 'auto'),
                arahArab: arab.map((el) => getComputedStyle(el).direction),
                scrollHorizontal: document.documentElement.scrollWidth > window.innerWidth + 1,
                tombolKecil: [...document.querySelectorAll('button, .btn')]
                    .filter((b) => b.offsetParent !== null)
                    .filter((b) => b.getBoundingClientRect().height < 32).length,
            };
        });
        console.log('[VERIFIKASI]', JSON.stringify(hasil));

        if (hasil.jumlahSoal === 0) throw new Error('Tidak ada soal yang tampil');
        if (!hasil.semuaDirAuto) throw new Error('Teks soal harus memakai dir="auto" (DESIGN §6)');
        if (hasil.soalArab > 0 && hasil.arahArab.some((d) => d !== 'rtl')) throw new Error('Teks Arab tidak tampil RTL');
        if (hasil.scrollHorizontal) throw new Error('Halaman ujian memiliki scroll horizontal di 360px');
        if (errors.length) throw new Error('Error JavaScript di halaman: ' + errors.join('; '));

        await page.screenshot({ path: path.join(outDir, 'ujian_hp_360.png'), fullPage: true });
        console.log('[TEST UJIAN HP] LULUS');
    } finally {
        await browser.close();
    }
}

main().catch((err) => {
    console.error('[TEST UJIAN HP GAGAL]', err.message || err);
    process.exit(1);
});
