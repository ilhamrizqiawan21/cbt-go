// Pratinjau soal untuk form admin/guru (T2.4): menampilkan isi kolom yang sedang
// diketik dengan font Arab dan arah teks otomatis. Memakai textContent (tanpa HTML).
(function () {
    const ARAB = /[؀-ۿݐ-ݿࢠ-ࣿﭐ-﷿ﹰ-ﻼ]/;
    let panel, body;

    function ensurePanel() {
        if (panel) return;
        panel = document.createElement('div');
        panel.id = 'pratinjau-soal';
        panel.hidden = true;
        const head = document.createElement('div');
        head.className = 'pj-head';
        head.textContent = 'Pratinjau';
        body = document.createElement('div');
        body.className = 'pj-body';
        body.setAttribute('dir', 'auto');
        panel.append(head, body);
        document.body.appendChild(panel);
    }

    function tampilkan(el) {
        ensurePanel();
        const teks = el.value || '';
        body.textContent = teks;
        body.classList.toggle('ar', ARAB.test(teks));
        panel.hidden = teks.trim() === '';
    }

    function target(e) {
        const el = e.target;
        return el && el.matches && el.matches('textarea[dir="auto"], input[dir="auto"]') ? el : null;
    }

    document.addEventListener('input', e => { const el = target(e); if (el) tampilkan(el); });
    document.addEventListener('focusin', e => { const el = target(e); if (el) tampilkan(el); });
    document.addEventListener('focusout', e => { if (target(e) && panel) panel.hidden = true; });
})();
