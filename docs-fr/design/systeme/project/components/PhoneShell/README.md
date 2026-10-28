# PhoneShell

The phone shell shows one pane at a time, with a tab bar at the bottom: Radio (home), Sky, Traffic, Right now.

**Markup.** `cl-phone` › `cl-phone__bar` (the sign, what is on air, UTC), `cl-phone__pane`, `cl-tabbar` of `cl-tabbar__tab`s (icon and word, `aria-selected`). Inside a phone frame, and on any screen under 768 px, controls take the touch sizes: `size-control-touch`, `size-target-touch`, `size-row-touch`.

**Rules.** The thread is home. Selecting an aircraft on the map moves to Traffic with its story. Settings hide the tab bar.

**Height.** The page is the visible height: `height: 100vh; height: 100dvh` on the body and the application's container (28/10). On iOS 26, Safari's bar floats over the bottom of a page of `100vh`, and the tab bar's words went under it: 836 px of page for 796 visible, measured in the iOS simulator (iPhone 17 Pro Max, iOS 26.5); with `100dvh`, 796 for 796. No safe-area margin is needed. A phone screen is measured in the iOS simulator: headless Chrome has no Safari bar.
