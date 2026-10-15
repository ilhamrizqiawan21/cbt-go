/**
 * CBT Sistem — Sidebar Toggle
 * Sesuai dengan struktur: #wrapper > nav.sidebar + #content-wrapper
 *
 * Desktop (>768px): sidebar collapse jadi icon-only (64px) ↔ full (240px)
 *                   #content-wrapper margin-left ikut berubah
 * Mobile  (≤768px): sidebar slide-in overlay, #content-wrapper tetap full-width
 */
(function () {
    'use strict';

    var MOBILE_BP   = 768;
    var STORAGE_KEY = 'cbt_sidebar';

    var wrapper     = document.getElementById('wrapper');
    var sidebar     = document.getElementById('sidebar') || document.querySelector('.sidebar');
    var backdrop    = document.getElementById('sidebarBackdrop');
    var toggleBtn   = document.getElementById('sidebarToggle');
    var toggleIcon  = document.getElementById('toggleIcon');

    if (!sidebar || !wrapper) return;

    /* ── helpers ── */
    function mobile() { return window.innerWidth <= MOBILE_BP; }
    function load()   { try { return localStorage.getItem(STORAGE_KEY); } catch(e) { return null; } }
    function save(v)  { try { localStorage.setItem(STORAGE_KEY, v); }    catch(e) {} }

    /* ── icon update ── */
    function setIcon(open) {
        if (!toggleIcon) return;
        toggleIcon.className = open ? 'bi bi-x-lg' : 'bi bi-list';
    }

    /* ══ DESKTOP: collapse / expand ══ */
    function desktopCollapse() {
        wrapper.classList.add('sidebar-collapsed');
        save('collapsed');
        setIcon(false);
    }
    function desktopExpand() {
        wrapper.classList.remove('sidebar-collapsed');
        save('open');
        setIcon(true);
    }
    function toggleDesktop() {
        wrapper.classList.contains('sidebar-collapsed') ? desktopExpand() : desktopCollapse();
    }

    /* ══ MOBILE: overlay open / close ══ */
    function mobileOpen() {
        sidebar.classList.add('sidebar-open');
        if (backdrop) backdrop.classList.add('show');
        document.body.style.overflow = 'hidden';
        setIcon(true);
    }
    function mobileClose() {
        sidebar.classList.remove('sidebar-open');
        if (backdrop) backdrop.classList.remove('show');
        document.body.style.overflow = '';
        setIcon(false);
    }
    function toggleMobile() {
        sidebar.classList.contains('sidebar-open') ? mobileClose() : mobileOpen();
    }

    /* ══ Main handler ══ */
    function handleToggle(e) {
        e.preventDefault();
        e.stopPropagation();
        mobile() ? toggleMobile() : toggleDesktop();
    }

    /* ── Event listeners ── */
    if (toggleBtn) toggleBtn.addEventListener('click', handleToggle);
    if (backdrop)  backdrop.addEventListener('click', mobileClose);

    document.addEventListener('keydown', function(e) {
        if (e.key === 'Escape' && mobile()) mobileClose();
    });

    /* ── Swipe gestures (touch) ── */
    var tx = 0, ty = 0;
    document.addEventListener('touchstart', function(e) {
        tx = e.touches[0].clientX;
        ty = e.touches[0].clientY;
    }, { passive: true });
    document.addEventListener('touchend', function(e) {
        if (!mobile()) return;
        var dx = e.changedTouches[0].clientX - tx;
        var dy = Math.abs(e.changedTouches[0].clientY - ty);
        if (dy > 80) return;
        if (!sidebar.classList.contains('sidebar-open') && tx < 30 && dx > 55) {
            mobileOpen();   /* swipe kanan dari tepi kiri → buka */
        } else if (sidebar.classList.contains('sidebar-open') && dx < -55) {
            mobileClose();  /* swipe kiri → tutup */
        }
    }, { passive: true });

    /* ── Resize handler ── */
    var resizeT;
    window.addEventListener('resize', function() {
        clearTimeout(resizeT);
        resizeT = setTimeout(function() {
            if (!mobile()) {
                mobileClose();
                load() === 'collapsed' ? desktopCollapse() : desktopExpand();
            } else {
                wrapper.classList.remove('sidebar-collapsed');
            }
        }, 150);
    });

    /* ── Init ── */
    (function init() {
        if (mobile()) {
            wrapper.classList.remove('sidebar-collapsed');
            mobileClose();
        } else {
            load() === 'collapsed' ? desktopCollapse() : desktopExpand();
        }
    })();

})();
