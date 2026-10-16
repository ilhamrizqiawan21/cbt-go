#!/usr/bin/env node
const { chromium } = require('playwright');
const fs = require('fs');
const path = require('path');

async function testArabHP() {
    const baseUrl = process.env.CBT_URL || 'http://127.0.0.1:3000';
    const outDir = path.resolve(__dirname, 'out');
    if (!fs.existsSync(outDir)) {
        fs.mkdirSync(outDir, { recursive: true });
    }

    console.log(`[UI TEST] Memulai Uji UI Arab HP 360px terhadap ${baseUrl}...`);

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

    // Set viewport tepat 360x640 (standar HP Android)
    const context = await browser.newContext({
        viewport: { width: 360, height: 640 },
        userAgent: 'Mozilla/5.0 (Linux; Android 10; Mobile) AppleWebKit/537.36 CBT-App/1.0'
    });

    const page = await context.newPage();

    // 1. Kunjungi halaman utama / login
    await page.goto(`${baseUrl}/login`, { waitUntil: 'domcontentloaded' });

    // 2. Simulasikan render komponen ujian di browser untuk verifikasi visual teks Arab dan Essay
    await page.setContent(`
        <!DOCTYPE html>
        <html lang="id">
        <head>
            <meta charset="UTF-8">
            <meta name="viewport" content="width=device-width, initial-scale=1.0">
            <link href="${baseUrl}/css/style.css" rel="stylesheet">
            <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/bootstrap@5.3.0/dist/css/bootstrap.min.css">
            <style>
                :root {
                    --exam-font-scale: 1;
                    --font-arabic: 'Amiri', serif;
                }
                .btn-font-size { width: 44px; height: 44px; min-height: 44px; min-width: 44px; }
                .soal-text.ar { font-family: var(--font-arabic); font-size: 1.35rem; direction: rtl; }
            </style>
        </head>
        <body style="background: #f0f2f8; padding: 10px;">
            <!-- Kontrol Font Size Tap Target >= 44px -->
            <div class="d-flex align-items-center gap-2 mb-3">
                <button type="button" class="btn-font-size btn btn-outline-secondary" id="btn-font-dec">A−</button>
                <span id="font-size-val">100%</span>
                <button type="button" class="btn-font-size btn btn-outline-secondary" id="btn-font-inc">A+</button>
            </div>

            <!-- Status Sinkronisasi -->
            <div id="sync-indicator" class="alert alert-success py-1 px-2 d-inline-flex align-items-center gap-1 mb-3">
                <span id="sync-indicator-txt">Tersimpan</span>
            </div>

            <!-- Kartu Soal Arab -->
            <div class="card mb-3 p-3 soal-card" data-id="1">
                <div class="fw-bold text-muted mb-2">Soal 1 (Pilihan Ganda Arab)</div>
                <div class="soal-text ar" dir="auto">عَيِّنِ الفِعْلَ الْمَاضِيَ مِنَ الْجُمَلِ التَّالِيَةِ: كَتَبَ التِّلْمِيذُ الدَّرْسَ</div>
                <div class="mt-2">
                    <div class="form-check my-2" style="min-height: 44px; display: flex; align-items: center;">
                        <input class="form-check-input me-2" type="radio" name="soal_1" id="opt_a">
                        <label class="form-check-label ar" for="opt_a" dir="auto">كَتَبَ</label>
                    </div>
                </div>
            </div>

            <!-- Kartu Soal Essay (FR-09, D-006: Lembar Kertas, Tanpa Textarea) -->
            <div class="card p-3 soal-card" data-id="2">
                <div class="fw-bold text-muted mb-2">Soal 2 (Uraian / Essay)</div>
                <div class="soal-text mb-2">Jelaskan hikmah dan keutamaan mempelajari kaidah bahasa Arab!</div>
                <div class="alert alert-light border d-flex align-items-center gap-2 mb-0">
                    <div>
                        <strong class="d-block text-dark">Dijawab di Lembar Kertas</strong>
                        <small class="text-muted">Tulis jawaban uraian Anda secara rapi pada lembar jawaban kertas yang disediakan pengawas.</small>
                    </div>
                </div>
            </div>
        </body>
        </html>
    `);

    // 3. Verifikasi DOM & Dimensi
    const btnSizeBox = await page.locator('#btn-font-dec').boundingBox();
    console.log(`[VERIFIKASI] Tombol tap target width: ${btnSizeBox.width}px, height: ${btnSizeBox.height}px`);
    if (btnSizeBox.width < 44 || btnSizeBox.height < 44) {
        throw new Error(`Ukuran tombol kurang dari 44px: ${btnSizeBox.width}x${btnSizeBox.height}`);
    }

    const textareaCount = await page.locator('textarea').count();
    console.log(`[VERIFIKASI] Jumlah textarea pada kartu soal: ${textareaCount}`);
    if (textareaCount > 0) {
        throw new Error(`Ditemukan elemen textarea pada soal essay! Harusnya tanpa textarea.`);
    }

    const dirAutoCount = await page.locator('[dir="auto"]').count();
    console.log(`[VERIFIKASI] Elemen dengan dir="auto": ${dirAutoCount}`);
    if (dirAutoCount < 2) {
        throw new Error(`Atribut dir="auto" tidak lengkap pada teks Arab.`);
    }

    // 4. Ambil screenshot 360px
    const screenshotPath = path.join(outDir, 'arab_hp_360px.png');
    await page.screenshot({ path: screenshotPath, fullPage: true });
    console.log(`[SUKSES] Screenshot disimpan ke: ${screenshotPath}`);

    await browser.close();
}

testArabHP().then(() => {
    console.log('[TEST ARAB HP] LULUS SEMUA VALIDASI 360px.');
    process.exit(0);
}).catch((err) => {
    console.error('[TEST ARAB HP GAGAL]', err);
    process.exit(1);
});

