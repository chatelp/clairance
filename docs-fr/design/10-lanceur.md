# 10 — Le lanceur, en Clairance natif macOS

*24/10/2026, session design, à la demande de l'agent du lanceur (« Plan lanceur de Clairance ») et
du propriétaire (`docs-fr/34-lanceur.md`). Étape 1, le gardien : l'icône et ses états, le menu, les
notifications, l'icône de l'application. La fenêtre de l'étape 2 viendra dans une fiche à part.
Validée par le propriétaire le 24/10 (relayé par l'agent du lanceur) ; l'étape 1b est codée sur cette forme.*

## Le principe : Clairance, dans les règles du Mac

Le lanceur n'est pas une page web dans une fenêtre. Il prend les conventions du Mac : menu de la
barre des menus (`MenuBarExtra`, style menu), notifications du système, police et contrôles du
système, clair et sombre suivis automatiquement. Il garde l'identité de Clairance là où elle se
voit :
- **le signe et ses états**, qui disent la santé de la production par la forme ;
- **le vocabulaire des voyants** : ● bon, ▲ défaut, ■ panne, – éteint, ◐ bascule. C'est celui de
  la page (`StatusLight`) : une forme et un mot, jamais une couleur seule ;
- **le ton** : ce qui s'est passé, la cause et depuis quand, ce qui marche encore. Ni « Error », ni
  « Failed », ni point d'exclamation, ni verdict ;
- **les nombres de la marque** : espace fine dans les milliers (« 1 200 »), unités après une
  espace (« 57 GB », « 2 min »), durées en « 2 h 14 min », fréquences à trois décimales.

Langue : **anglais**, comme le menu actuel et toute l'interface de Clairance. Heures : celles du Mac
(« 14:05 ») ; c'est un outil de la machine, pas de l'aviation. Partout où c'est possible, une
durée (« for 3 min ») plutôt qu'une heure.

## L'icône : six états

Les fichiers sont dans `docs-fr/design/identite/`, en 18 pt, à copier dans `tools/menubar/`.

| État | Dessin | Image | Quand |
|---|---|---|---|
| **Stopped** | l'écran vide et son centre, à 55 % | modèle (`barre-menus-arret.svg`) | arrêtée par choix ; ce n'est jamais une panne |
| **Switching** | l'écran et ◐ au centre | modèle (`barre-menus-bascule.svg`) | un démarrage ou un arrêt demandé au lanceur, le temps qu'il aboutisse (15 s au plus) |
| **Running** | le balayage et son plot | modèle (`barre-menus-marche.svg`) | elle tourne, tous les contrôles passent, rien n'est à l'antenne de la station |
| **On air** | l'écran allumé, en vert | couleur (`barre-menus-ecoute.svg`) | elle tourne, tous les contrôles passent, la station a des fréquences à l'antenne |
| **Fault** | ▲ ambre dans l'écran, sur disque sombre | couleur (`barre-menus-defaut.svg`) | elle tourne, mais un contrôle échoue (liste ci-dessous) |
| **Down** | ■ rouge dans l'écran, sur disque sombre | couleur (`barre-menus-panne.svg`) | elle devrait tourner et ne tourne pas : arrêtée d'elle-même, ou serveur muet |

**Priorité** : Switching, puis Down, puis Fault, puis On air, puis Running, puis Stopped. Une
production en défaut n'a jamais l'air à l'écoute : c'est exactement ce qui a trompé le 16-18/10.
Il n'y a qu'un état de défaut, qu'il y ait des fréquences à l'antenne ou non : le menu dit le reste.

- **Ce qui met en défaut** : la station injoignable (les deux causes), l'audio muet sur une
  fréquence à l'antenne, la liaison `radio-ctl-sync` arrêtée, la transcription muette ou dégradée,
  rien de transcrit depuis N minutes alors que l'audio arrive, le disque d'archive absent, le
  disque interne sous 20 Go, la pression mémoire en avertissement ou critique, la production
  lancée hors du lanceur.
- **Ce qui ne change pas l'icône** : une instance d'essai oubliée, un assistant sans instance. Ils
  ne sont pas la production ; ils ont leur ligne au menu et leur notification.

Les trois états en couleur suivent la règle d'« On air » (`05-identite.md`) : un disque sombre
(`surface.selected`, `surface.warn`, `surface.urgent`) les garde lisibles sur une barre claire
comme sombre, et macOS ne les teinte pas. Les trois autres restent des images modèle. Rien ne
bouge ni ne clignote : l'état change d'un coup.

**Pour VoiceOver**, le libellé du bouton suit l'état : « Clairance: production stopped »,
« … starting », « … stopping », « … running », « … running, on air », « … running with a
fault: <première cause> », « … down: <cause> ».

## L'icône de l'application

`identite/app-icon.svg`, 1024, au gabarit des icônes du Mac : une tuile de 824 à coins de 185,
sur une toile de 1024, avec son ombre. C'est la tuile sombre du favicon, avec une légère lumière
venue du haut, et le signe entier (anneau de distance compris) à 512. Elle sert au Finder, au Dock
quand la fenêtre de l'étape 2 sera ouverte, et à **chaque notification**. Pour l'exporter :
`rsvg-convert -w N app-icon.svg` pour 16, 32, 64, 128, 256, 512 et 1024 (et les @2x), puis
`iconutil -c icns`.

## Le menu

Style menu natif (`MenuBarExtra(...).menuBarExtraStyle(.menu)`). Les lignes d'information sont
des éléments inactifs, en gris, comme le font les menus du système. Les sections portent un
en-tête (`Section`, rendu en en-tête à partir de macOS 14). Dans les valeurs, chiffres à chasse
fixe (`.monospacedDigit()`). Pas de point final sur une ligne de menu.

```
Production running · 2 h 14 min
──────────────────────────────────────────
HEALTH
● Station · on air 124.350, 126.425
● ADS-B · 107 aircraft, 700 msg/s
● Audio · 2 frequencies, both receiving
● Transcription · queue 0, 1.0 s a transmission
● Disk · 212 GB free · archive 57 of 60 GB
● Memory · normal
──────────────────────────────────────────
Stop production
Restart production
Open Clairance                              ⌘O
Copy remote address
Show production log                         ⌘L
──────────────────────────────────────────
INSTANCES
Production · port 8000                       ›
Test · port 8020 · stops at 18:00            ›
──────────────────────────────────────────
Quit launcher…                              ⌘Q
```

**La première ligne** dit l'état, et depuis quand :
- `Production stopped` ;
- `Production starting…` et `Production stopping…` ;
- `Production running · 2 h 14 min` ;
- `▲ Production deaf · 3 min` ;
- `■ Production down · stopped by itself 3 min ago`, ou `■ Production not answering · 1 min`.

**La cause**, en défaut ou en panne : une seconde ligne, sous la première, en une phrase de 60
caractères au plus. Si elle est plus longue, elle se coupe en deux lignes, jamais en ellipse : une
cause tronquée ne sert à rien.

```
▲ Production deaf · 3 min
The station answers this Mac, not co-atc:
macOS took the local network from co-atc
```

**Sans application responsable** (mesuré par l'agent du lanceur le 24/10) : quand l'application
qui a lancé la production se ferme (le lanceur, un terminal, Claude), la production continue de
tourner, mais macOS lui retire aussitôt le réseau local, et pour de bon. Le menu le dit sans
attendre que le contrôle de la station s'en aperçoive :

```
▲ Production without local network
The app that started it quit: macOS took its local network
Restart from Clairance
```

**Lancée hors du lanceur** : une ligne d'avertissement, juste sous l'état, avec l'action
juste après :

```
▲ Started from Terminal: it stops hearing if Terminal quits
Restart from Clairance
```

**Les lignes de santé** suivent la forme des voyants : ● bon, ▲ défaut, ■ panne, ○ inconnu (pas de
réponse du serveur), – éteint (pas de transcription configurée). Chacune donne un nom, « · », puis
le fait. En défaut, le fait devient la cause courte :
- `▲ Station · not reached for 3 min (no route to host)` ;
- `▲ ADS-B · nothing read for 3 min` ;
- `▲ Audio · 124.350 silent for 2 min` ;
- `▲ Transcription · sidecar silent for 1 min, 12 waiting` ;
- `▲ Transcription · French model not loaded` ;
- `▲ Disk · 18 GB free` ;
- `▲ Memory · pressure warning` ;
- `– Transcription · off`.

**Les actions** dépendent de l'état : `Start production` à l'arrêt ; `Stop production` et
`Restart production` en marche ; aucune pendant la bascule. `Open Clairance` est inactif quand le
serveur ne répond pas.

**Les instances** : une ligne par instance, avec un sous-menu, comme aujourd'hui :
- la production : `Production · port 8000` ;
- une instance d'essai : `Test · port 8020 · stops at 18:00`, puis, passé l'échéance,
  `▲ Test · port 8020 · 2 h past its end` ;
- un assistant sans instance : `▲ Sidecar without an instance · 9 GB` (ou `Station link without an
  instance`).

Dans le détail d'une instance, les deux lignes nouvelles de l'étape 1a :
- `Depends on: Clairance`, ou `Depends on: Terminal`, `Depends on: claude` : l'application dont
  elle tient le réseau local, au nom que macOS lui donne ; et quand cette application s'est
  fermée : `Depends on: no app · no local network` ;
- `Stops at 18:00 (in 2 h 10 min)`, ou `Was to stop at 18:00 (2 h ago)`.

## Quitter le lanceur

Le lanceur est l'application responsable de ce qu'il démarre : s'il se ferme, la production
continue sans réseau local. Le menu le dit donc :
- `Quit launcher` (⌘Q), sans alerte, quand rien ne dépend de lui ;
- `Quit launcher…` (⌘Q), avec les points de suspension des commandes qui demandent une
  confirmation, quand une instance en dépend.

L'alerte (`NSAlert`, style avertissement, l'icône de l'application) :

| | |
|---|---|
| Titre | Quit the launcher? |
| Texte, la production seule | Production runs from the launcher. If it quits, production keeps running without the local network: no station, nothing transcribed. |
| Texte, plusieurs instances | Production and the test instance on port 8020 run from the launcher. If it quits, they keep running without the local network: no station, nothing transcribed. |
| Texte, une instance d'essai seule | The test instance on port 8013 runs from the launcher. If it quits, it keeps running without the local network: no station, nothing transcribed. |
| Boutons | **Cancel** (par défaut, Retour et Échap) · le bouton d'arrêt · **Quit Anyway** |
| Le bouton d'arrêt nomme ce qu'il arrête | la production seule : **Stop Production and Quit** ; une instance d'essai seule : **Stop Instance and Quit** ; plusieurs : **Stop All and Quit** |

« Stop Production and Quit » arrête proprement ce qui dépend du lanceur, puis le ferme : c'est la
sortie propre, celle qu'on cherche le plus souvent en quittant. « Quit Anyway » garde le sens
d'aujourd'hui, en le nommant pour ce qu'il est. Les titres des boutons suivent la casse des boutons
du Mac (chaque mot en capitale).

**À venir, le porteur** (choix du propriétaire, 24/10) : la production tournera dans un processus
« porteur » séparé du menu, si bien que le lanceur pourra se fermer ou se mettre à jour sans la
couper. Alors l'alerte ne concernera plus la production, seulement les instances qui dépendent du
menu lui-même ; l'en-tête « Production without local network » restera, pour une production lancée
à l'ancienne. Les textes changeront après la mesure de l'agent, pas avant.

**Relancer depuis le lanceur** (« Restart from Clairance », et « Restart production » du menu)
passe par une confirmation, comme tout ce qui interrompt la production :

| | |
|---|---|
| Titre | Restart production from the launcher? |
| Texte | It stops, then starts again from the launcher. Nothing is transcribed in between. |
| Boutons | **Restart** (par défaut) · **Cancel** (Échap) |

## Les notifications

Une par incident, à son début. Quand il cesse, une seconde notification **remplace** la première
(même identifiant de requête) : le centre de notifications garde une ligne par incident, et rien
ne se répète entre les deux. Elles sont groupées par sorte d'incident (`threadIdentifier`).

Chacune a un titre (l'état, en quelques mots), un corps (la cause en une phrase, puis ce qui marche
encore ou ce qu'il faut faire) et un clic. Le son et le niveau suivent la gravité :
- **panne, et production sourde** : son, niveau « sensible au temps » (traverse un mode de
  concentration qui l'autorise) ;
- **défaut** : sans son, niveau actif ;
- **retour à la normale, événement ponctuel** : sans son, niveau passif.

| Incident | Titre | Corps | Clic, boutons | Son |
|---|---|---|---|---|
| co-atc arrêté de lui-même | Production stopped by itself | co-atc exited at 14:05 (status 2). Nothing is transcribed or archived until it starts again. | journal ; Start production, Show log | oui |
| … rétabli | Production running again | Started at 14:11, after 6 min stopped. | menu | — |
| le serveur ne répond plus | Production not answering | co-atc is running but its server has not answered for 1 min. The page and the map are frozen. | journal ; Restart production, Show log | oui |
| … rétabli | Production answering again | After 3 min without an answer. | Clairance | — |
| réseau local retiré | Production deaf: no local network | The station answers this Mac but not co-atc: macOS has taken the local network from it. Restarting it from Clairance gives it back. | journal ; Restart production | oui |
| la station ne répond pas | Production deaf: the station does not answer | Neither co-atc nor this Mac has reached the station for 2 min (no route to host). ADS-B and audio stop until it answers. | journal | oui |
| … rétabli (les deux) | Production hears the station again | After 34 min deaf. | Clairance | — |
| audio muet sur une fréquence | No audio on 124.350 | Approche De Gaulle has sent nothing for 2 min while on air at the station. The other frequency is receiving. | Clairance | — |
| … rétabli | Audio back on 124.350 | After 5 min silent. | Clairance | — |
| liaison station arrêtée | Station link stopped | radio-ctl-sync exited: Clairance no longer follows what the station puts on air. The current streams keep playing. | journal | — |
| … rétabli | Station link running again | After 12 min. | menu | — |
| sidecar muet | Transcription silent | The sidecar has not answered for 1 min. 12 transmissions are waiting in its queue. | journal | — |
| … rétabli | Transcription back | After 4 min silent. | Clairance | — |
| langue dégradée | French transcription unavailable | The French model is not loaded: transmissions are read by the English model only. | journal | — |
| … rétabli | French transcription back | After 20 min. | — | — |
| sidecar relancé (ponctuel) | Transcription restarted | The sidecar stopped and co-atc started it again 40 s later. | journal | — |
| rien de transcrit | Nothing transcribed for 20 min | Audio arrives on 2 frequencies on air, and no transmission has been cut since 13:40. | Clairance | — |
| … rétabli | Transmissions again | After 26 min without one. | Clairance | — |
| disque d'archive absent | Archive disk missing | “Archive” is not mounted: the audio archive is not written. Transcription continues. | Finder ; Show in Finder | — |
| … rétabli | Archive disk back | After 2 h. | — | — |
| disque interne, 20 Go | Disk almost full: 18 GB left | The internal disk has less than 20 GB free. The archive uses 57 GB of its 60 GB. | Finder, le dossier des données | — |
| disque interne, 5 Go | Disk nearly full: 4 GB left | Less than 5 GB free: the databases may stop being written. | Finder, le dossier des données | oui |
| … rétabli | Disk space back: 41 GB free | — | — | — |
| pression mémoire | Memory pressure: warning (ou critical) | macOS is short of memory. The sidecar uses 17 GB, co-atc 1.2 GB. | Moniteur d'activité | critique seulement |
| … rétabli | Memory pressure normal | After 8 min. | — | — |
| lancée hors du lanceur | Production depends on Terminal | It was started from Terminal: if Terminal quits or updates, production loses the local network. Restart it from Clairance to keep it. | menu ; Restart from Clairance | — |
| … rétabli | Production runs from Clairance | It no longer depends on Terminal. | — | — |
| instance d'essai oubliée | Test instance still running | Port 8020 has run for 2 d 4 h, 46 h past its end. It still listens to the station's streams. | menu ; Stop it… | — |
| assistant sans instance | Sidecar left without an instance | A transcription sidecar (PID 8123) runs with no co-atc and uses 9 GB. | menu ; Stop it… | — |

Règles d'écriture :
- **Le titre dit l'état** et le nom de la chose (« Production », « Transcription », « Disk »), pas
  une erreur.
- **Le corps dit un fait vérifié**, rien d'autre : les chiffres sont ceux que le contrôle a
  mesurés, et une hypothèse ne s'écrit pas. « Nothing transcribed » ne dit pas pourquoi : le ciel
  peut être calme.
- **« Depends on » et « Started from »** nomment l'application telle que macOS la nomme
  (Terminal, iTerm, Claude, Visual Studio Code).
- **Un bouton qui arrête ou relance** (« Stop it… », « Restart production ») passe par la même
  confirmation qu'au menu. Une notification ne fait jamais rien seule.
- **« Clairance »** comme cible de clic veut dire : la page, si le serveur répond ; sinon le menu.
- **« menu »** veut dire : ouvrir le menu du lanceur, si l'agent le peut (un `NSStatusItem` géré
  par le lanceur le permet, pas `MenuBarExtra` seul) ; sinon, le clic n'ouvre rien d'autre que
  la notification elle-même. « journal » ouvre le journal de la production à sa fin ; « Finder »
  montre le volume ou le dossier des données ; « Moniteur d'activité » ouvre l'application.

## Ce qui reste à l'étape 2

La fenêtre du tableau de bord : les blocs de `34-lanceur.md`, avec les mêmes formes, les mêmes
nombres et le même ton. Sa fiche viendra à la demande de l'agent.
