# FolderPicker

Choosing a folder **of the server's machine** from the page (owner, 28/10): the database archive, Settings › Server › "Archive old databases to". A page cannot open the system's folder window for the server: the browser gives no path, and the page often runs on another machine. So the page browses the server's folders itself, for an admin only (`GET /api/v1/server/folders?path=`).

**The button.** `Choose…` (`cl-btn`, `aria-expanded`) right of the path field, both in a `cl-pathfield`: one line, the field takes what is left (380 px on a computer, the whole width on a phone). The field stays: a path can still be typed.

**A panel, not a window.** The picker unfolds under its field, inside the page (`cl-folder`, on `surface-panel`, `radius-lg`), at the fields' reading width; the field's light stays above it. Never a second window over the settings window: one window at a time, and the same panel serves the phone.

**Where it opens.** At the field's folder; if that cannot be read (its disk not plugged in), at the nearest folder above it that can; with no archive, at the disks (`/Volumes`), or else the home folder.

**Its parts**, top to bottom:
- **Places** (`cl-folder__places`): the disks of `/Volumes`, each with its room ("Archive · 1.8 TB free", `cl-folder__free`), then Home, without; the place that holds the open folder in `aria-current`. A click opens it.
- **The path** (`cl-folder__crumbs`, a `nav` "Open folder"): each part of the open folder's path is a button back up to it; the last in `aria-current="location"`, strong. A long path wraps; nothing is elided.
- **The folders inside** (`cl-folder__list`): one row per folder (`cl-folder__row`: the folder glyph, its name, a chevron), sorted by name; a click opens it. Six rows, then the list scrolls. Files and hidden folders never appear: the server leaves them out. A folder that cannot be chosen still opens (one goes through `/Volumes`, which nobody can write, to reach a disk), with why at its right, in words (`cl-folder__note`): "read only", "the databases". Never greyed: grey would say it cannot be opened.
- **The foot** (`cl-folder__foot`): what choosing the open folder means, then Cancel (`cl-btn--quiet`) and **Choose this folder** (`cl-btn--primary`). The folder chosen is the open one: no selection apart from opening.

**The foot's line**, from the server's `here` (`choosable`, `why`):
- can be chosen: `cl-plain` "1.8 TB free on Archive." (the room of the place that holds it);
- the databases' own folder, or one inside it: `StatusLight --warn` "The databases are kept here: choose a folder outside, best on another disk."; Choose disabled;
- one the server cannot write: `--warn` "The server cannot write in this folder: choose another."; Choose disabled.

**The list's states**: "Reading…" (`cl-light--switching`) while it loads; "No folder inside." (`cl-folder__empty`); a folder that cannot be read, `--bad` "This folder cannot be read: is its disk plugged in?", the places and the path still there to go elsewhere. More than 500 folders: the list ends with "The first 500 folders. Type the path in the field for another." (the server should sort before cutting, and say that it cut).

**Choosing.** Choose this folder writes the path in the field and saves it as a typed path does (the server checks it again); the panel folds, the focus goes back to Choose…, the field's light gives the result. Cancel folds the panel and changes nothing.

**Keyboard.** At opening, the focus on the first row (on the path's last part when the folder is empty). ↑ ↓ between rows, Home and End to the first and last, Enter or → opens the row's folder, ← or Backspace goes up one folder; Tab reaches the places, the path and the foot. **Escape folds the panel** and gives the focus back to Choose…: the innermost first, the settings window stays open.

**Phone.** The same panel at the page's width: the places as 44 px buttons that wrap, the rows 44 px, the path's parts 44 px high, Cancel and Choose this folder on their own line, at the right.

**Who.** An admin only: a viewer does not see the Server page, and the server answers 403 to anyone else.
