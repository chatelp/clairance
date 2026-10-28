// The tab's icon follows the listening (docs-fr/design/05-identite.md, "Trois
// états du même signe"): the scope lights up while at least one frequency is
// listened to in this browser and its sound actually runs -- not while the
// browser holds it suspended, waiting for a gesture (design review, 26/10) --
// and goes back to the plain scope otherwise. No animation: the sign does not
// move.
(function () {
    'use strict';

    const QUIET = 'brand/favicon.svg';
    const LISTENING = 'brand/favicon-actif.svg';

    function apply(listening) {
        const link = document.querySelector('link[rel="icon"][type="image/svg+xml"]');
        if (!link) return;
        const href = listening ? LISTENING : QUIET;
        if (link.getAttribute('href') !== href) link.setAttribute('href', href);
    }

    document.addEventListener('alpine:initialized', () => {
        if (!window.Alpine || !Alpine.store('atc')) return;
        Alpine.effect(() => apply(!!Alpine.store('atc').anyUnmuted && !!Alpine.store('atc').audioRunning));
    });
})();
