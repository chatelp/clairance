"""The page's own notices (28/10): a new version to load, and what is new.

Reference screens built by ecrans.py: the update notice while listening, the
"New version" button left in the top bar after "Later", the "What's new" card,
and the update notice on a phone. Only cl-* classes. The texts are proposals.
"""

RELOAD = ('<svg viewBox="0 0 16 16" width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" aria-hidden="true">'
          '<path d="M13 8a5 5 0 1 1-1.5-3.55"/><path d="M13 2.5v3h-3"/></svg>')


def update(listening=True):
    text = 'Reload this page to use the new version.'
    if listening:
        text += ' You are listening to 124.350: the sound stops for a moment, and a click starts it again.'
    return (f'<div class="cl-notice" role="status" aria-live="polite"><div class="cl-notice__head">{RELOAD}'
            f'<h2 class="cl-notice__title">Clairance has been updated</h2></div><p class="cl-notice__text">{text}</p>'
            '<div class="cl-notice__actions"><button class="cl-btn cl-btn--primary">Reload</button><button class="cl-btn cl-btn--quiet">Later</button></div></div>')


def whats_new(sign):
    items = ['The settings are now in pages, grouped by who they concern.',
             'The sound starts within a second, and plays on an iPhone.',
             'After a cut, the page reconnects on its own and says so.']
    return (f'<div class="cl-notice" role="status" aria-live="polite"><div class="cl-notice__head">{sign}'
            '<h2 class="cl-notice__title">What\'s new</h2><span class="cl-notice__when">28 October</span></div>'
            '<ul class="cl-notice__list">' + ''.join(f'<li>{i}</li>' for i in items) + '</ul>'
            '<div class="cl-notice__actions"><button class="cl-btn">OK</button><a class="cl-link" href="#">All the news</a></div></div>')


def later_button():
    return (f'<button class="cl-btn" aria-label="New version: reload the page to use it" data-help-title="New version" '
            f'data-help="Clairance has been updated. Reload this page to use the new version.">{RELOAD}New version</button>')
