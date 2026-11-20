# 11 — La fenêtre du lanceur (étape 2)

*20/11/2026, session design, à la demande de la session principale (étape 2 de `docs-fr/34-lanceur.md`).
Le propriétaire, le 20/11 : « améliorer l'app pour qu'elle indique qui est connecté au serveur — et aussi ne pas
tout passer par le menu déroulant, il faudrait une vraie fenêtre », « en respectant la charte graphique si
possible (voir avec l'agent design) mais ça doit rester une app franchement macOS ». Validée par le propriétaire
le 20/11 (D89), codée le jour même (acfdc2b, 360b656, 7fc07de), relue sur captures de la vraie application : six
retouches demandées (« Au codage », en fin de fiche). Maquette approchée : `docs-fr/design/lanceur/`
(`tools/design/lanceur.py`) ; la vraie fenêtre prend les contrôles du système, que la maquette imite.*

## Le principe

Le même que pour le menu (`10-lanceur.md`) : une application Mac, avec les scènes, les contrôles, la police et
les couleurs du système, le clair et le sombre suivis. L'identité de Clairance tient en quatre choses, comme dans
le menu : **le signe et ses six états**, **les formes des voyants** (● ▲ ■ – ○ ◐, une forme et un mot, jamais une
couleur seule), **le ton** (ce qui se passe, depuis quand, ce qui marche encore ; jamais « Error ») et **les
nombres de la marque** (« 2 h 14 min », « 1 200 », fréquences à trois décimales). Les couleurs des voyants
sont celles du système (vert, orange, rouge), qui s'accordent au clair, au sombre et au contraste accru. Les
mots de la fenêtre sont ceux du menu : une même chose ne porte jamais deux noms.

## La fenêtre

- **Une seule fenêtre**, titrée **« Dashboard »** : une `NSWindow` et sa `NSToolbar` qui portent la vue SwiftUI
  (une scène `Window` s'ouvrirait d'elle-même à chaque lancement). Taille par défaut 600 × 680 points (à 560, les
  trois boutons de la barre, 316 points, ne tenaient plus à côté du titre), minimum 480 × 420, redimensionnable ;
  macOS retient son cadre. Fermer la fenêtre (⌘W) ne quitte rien :
  le lanceur reste dans la barre des menus.
- **Une colonne**, un formulaire groupé (`Form { Section … }.formStyle(.grouped)`), qui défile. **Pas de barre
  latérale** : la fenêtre doit dire d'un coup d'œil où en est Clairance, et ses quatre sections tiennent dans la
  hauteur d'un écran. Une barre latérale découperait ce coup d'œil en pages. Elle viendra à l'étape 3, quand le
  journal et les ressources (une courbe sur 24 h) arriveront : chacun aura sa vue, et celle-ci deviendra
  « Overview », la première.
- **La barre d'outils** (unifiée, les boutons titrés et illustrés, symboles SF) porte les actions sur la
  production, à droite du titre :
  - à l'arrêt : **Start** (`play.fill`) ;
  - en marche : **Stop** (`stop.fill`) et **Restart** (`arrow.clockwise`) ;
  - pendant une bascule : les mêmes, désactivés ;
  - toujours : **Open Clairance** (`arrow.up.forward.app`), désactivé quand le serveur ne répond pas.

  Stop et Restart passent par une confirmation, comme au menu. Celle de Restart est dans `10-lanceur.md`. Celle
  de Stop est nouvelle ; elle vaut aussi pour « Stop production » du menu, qui arrête aujourd'hui sans
  demander :

  | | |
  |---|---|
  | Titre | Stop production? |
  | Texte | Nothing is transcribed or archived until it starts again. Listeners lose the sound and the page. |
  | Boutons | **Stop** (par défaut) · **Cancel** (Échap) |

## L'en-tête

En haut du contenu, hors des sections, comme l'en-tête d'une fiche « À propos » :

- **le signe à 48 points**, dans l'état de l'icône du menu (les six dessins de `identite/`, les trois en couleur
  gardant leur disque sombre) ;
- **la ligne d'état**, en titre (`.title2`, semi-gras), mot pour mot la première ligne du menu :
  « Production running · 2 h 14 min », « ▲ Production deaf · 3 min », « ■ Production did not start »… ;
- **la cause**, en défaut ou en panne, en texte secondaire, mot pour mot celle du menu, sur autant de lignes
  qu'il faut, jamais coupée d'une ellipse ;
- **les actions du moment**, sous la cause, en boutons ordinaires, comme au menu : « Restart from Clairance »
  (production lancée hors du lanceur, ou sans réseau local), « Start production » et « Leave stopped » (démarrage
  manqué) ;
- **la version**, en texte secondaire : « Version of 20 November 2026 · 9a4b5ce », suivie de « · up to date »,
  ou de « · 4 commits behind local » quand `repository.commits_behind` le dit (rien quand le champ est absent).
  Le commit en chasse fixe, sélectionnable.

## Les sections

Dans cet ordre. Chaque ligne est un `LabeledContent` : le nom à gauche, le fait à droite, en texte
secondaire, chiffres à chasse fixe (`.monospacedDigit()`) ; un fait long passe à la ligne.

### Permissions — seulement quand il en manque une

En tête des sections, parce qu'il faut agir. Une ligne par autorisation manquante, les mots du menu, le bouton à
droite :

```
▲ Clairance Server · local network not allowed      [Open Local Network Settings…]
▲ Clairance · notifications not allowed             [Open Notification Settings…]
```

Deux du même panneau : deux lignes, un seul bouton (sur la dernière).

### Health

Les six lignes du menu, dans le même ordre. À gauche, la forme du voyant (en couleur du système : ● vert,
▲ orange, ■ rouge ; – ○ ◐ en gris) puis le nom ; à droite, le fait de la ligne du menu, l'initiale en capitale :

```
● Station          On air 124.350, 126.425
● ADS-B            107 aircraft, 700 msg/s
● Audio            2 frequencies, both receiving
● Transcription    Queue 0, 1.0 s a transmission
● Disk             212 GB free · archive 57 of 60 GB
● Memory           Normal
```

En défaut, le fait devient la cause courte du menu (« ▲ Transcription · Sidecar silent for 1 min, 12 waiting »).
Production arrêtée : les lignes que le lanceur mesure lui-même gardent leur fait (la station vue de ce Mac, le
disque, la mémoire) ; les autres disent « – Production stopped ».

### Connected

L'en-tête de section donne le nombre : « Connected · 2 ». Une ligne par compte (`present[]`), triées par nom :
le nom du compte en semi-gras, puis une ou deux lignes en texte secondaire :

- les pages : « 2 pages open for 2 h 14 min » (depuis la plus ancienne, `since`), « 1 page open for 5 min » ;
  sans page : « No page open » ;
- le son : « Listening to 124.350 for 40 min », « Listening to 124.350 for 40 min, 126.425 for 5 min » ; sans
  flux : « · sound off » au bout de la ligne des pages.

```
pierre      2 pages open for 2 h 14 min
            Listening to 124.350 for 40 min
famille     1 page open for 5 min · sound off
```

Des durées, pas des heures (« for 40 min ») : on lit d'un coup d'œil qui est là depuis longtemps.

**Les états vides**, une ligne en texte secondaire à la place des comptes, chacune pour une raison distincte :
- personne : « No one connected » ;
- production arrêtée : « No one: production is stopped » ;
- serveur muet : « ○ Not known: the server does not answer » ;
- connexion non obligatoire, ou suivi de l'usage éteint : « Not known: Clairance opens without an account here ».
  `present[]` est alors vide comme quand il n'y a personne ; `health/detail` dit lequel des deux par
  `accounts.sign_in` et `accounts.usage` (ajoutés le 20/11, 7db54f8). « No one connected » seulement quand les deux
  sont vrais et la liste vide ; « Not known… » dès que l'un est faux ;
- serveur d'avant le 20/11, qui n'a pas ces champs : « Not known: this version of Clairance does not say » (ajouté
  au codage).

### Instances

Une ligne par instance, avec le titre de son entrée de menu d'aujourd'hui (« Production · port 8000 »,
« Test · port 8020 · stops at 18:00 », « ▲ Test · port 8020 · 2 h past its end », « ▲ Sidecar without an
instance · 9 GB »). Ce sont des groupes dépliants (`DisclosureGroup`), repliés à l'ouverture. Dépliée, une
instance montre :
- ses lignes de détail du menu, en paires nom / valeur : Page, Server, Transcription, Station link, Depends on,
  Stops at, Ports, Config, Databases (les chemins en chasse fixe, coupés au milieu s'il le faut :
  `.truncationMode(.middle)`, le chemin entier dans l'infobulle) ;
- une rangée de boutons, à la casse des boutons du Mac : **Open in Browser**, **Copy Address**, **Show Log**,
  **Show Config** ; pour la production, **Copy Remote Address** quand Tailscale est réglé ; pour une instance
  d'essai, **Stop Instance…** à droite, avec sa confirmation d'aujourd'hui. Un assistant sans instance :
  **Stop It…**.

Sans instance : « No instance running ».

## Le menu, allégé

```
Production running · 2 h 14 min
2 connected · pierre, famille
──────────────────────────────────────────
Show Dashboard                              ⌘0
Open Clairance                              ⌘O
Show production log                         ⌘L
──────────────────────────────────────────
Stop production…
Restart production…
──────────────────────────────────────────
Settings…                                   ⌘,
Quit launcher                               ⌘Q
```

- **L'état et sa cause**, comme aujourd'hui ; puis les lignes d'autorisation manquante, comme aujourd'hui.
- **La santé ne garde que ce qui demande un regard** : les lignes qui ne sont pas ● (▲ ■ ○), sous l'état. Quand
  tout va bien, aucune : l'icône le dit déjà. Le détail est dans la fenêtre.
- **« 2 connected · pierre, famille »** quand quelqu'un est là : le nombre, puis les noms, trois au plus
  (« 5 connected · pierre, famille, club and 2 more »). Un clic ouvre la fenêtre. Personne : pas de ligne.
- **Les noms des deux entrées.** « Show Dashboard » ouvre la fenêtre ; « Open Clairance » ouvre la page, comme
  aujourd'hui, avec son ⌘O. « Show Clairance » et « Open Clairance » se ressemblaient trop. Et partout dans le
  lanceur, « Clairance » désigne déjà la page : c'est la cible de clic des notifications (`10-lanceur.md`).
  « Show » est le verbe du Mac pour une fenêtre ; ⌘0 est le raccourci d'usage pour la fenêtre principale.
- **Stop et Restart prennent des points de suspension** : ils demandent maintenant une confirmation.
- **Partent dans la fenêtre** : les lignes de santé en bon état, les instances et leurs sous-menus, « Copy
  remote address ». **« Show production log » reste** : c'est le premier geste quand l'icône passe en défaut.

## Le Dock et le lancement

- **Un clic sur l'icône du Dock** ouvre la fenêtre, ou la ramène devant : c'est le geste de toute application
  qui rouvre sa fenêtre principale. Les réglages restent sous « Settings… » (⌘,), dans le menu et dans le menu
  de l'application.
- **Au lancement**, la fenêtre s'ouvre quand le propriétaire ouvre l'application (Finder, Dock, Spotlight).
  Elle ne s'ouvre pas au démarrage de session, si ce démarrage est un jour décidé (`keyAELaunchedAsLogInItem`).
  Le porteur (« Clairance Server ») n'ouvre jamais de fenêtre.
- **Le menu Fenêtre** de l'application liste « Dashboard », comme le fait le système.

## Ce qui bouge

La fenêtre relit l'état toutes les 5 s quand elle est visible (la réponse de `health/detail` a 5 s au plus), et
rien quand elle est cachée ou fermée. Les durées avancent à la minute. Rien ne s'anime : une valeur change d'un
coup, une ligne apparaît ou disparaît.

## Pour VoiceOver

Chaque ligne se lit nom puis fait. La forme d'un voyant a son mot pour libellé : « OK », « fault », « down »,
« unknown », « off », « switching ». Le signe de l'en-tête a le libellé de l'icône du menu (« Clairance:
production running, on air »).

## Au codage (20/11)

Relu sur les captures de la vraie application (`runs/lanceur/fenetre/captures/`, 01 à 10, dont la production
réelle), avec `clairance --status` et `health/detail` lus à côté.

**Écarts acceptés**, avec leur raison (détail dans `docs-fr/34-lanceur.md`, « L'étape 2 ») :
1. 600 points de large par défaut, et non 560 (« La fenêtre », plus haut).
2. `NSWindow` et `NSToolbar` plutôt qu'une scène `Window`.
3. Une ligne **Activity** dans le détail d'une instance : « 50 aircraft · 1 129 transmissions today · 37 %
   matched ». La ligne Server dit « Answering » ou « Not answering » quand le serveur ne donne pas sa version.
4. Un cinquième état vide pour Connected : un serveur d'avant le 20/11 ne dit pas qui est là (voir R6).
5. Le menu Fenêtre porte « Show Dashboard » (⌘0) en plus de la fenêtre que le système liste.
6. Pas de filet entre les lignes d'une instance dépliée : c'est le rendu d'un `DisclosureGroup` dans un
   formulaire groupé (voir R4).

**Retouches demandées** :
- **R1. Une seule source pour la version.** L'en-tête disait « acfdc2b · up to date » et la ligne Server
  « 1 commit behind local » dans la même fenêtre. L'en-tête lit `repository.commits_behind`, que le serveur relit
  une fois par minute. La ligne Server lançait git elle-même. Les deux lignes prennent la même lecture. La ligne
  Server perd « dev », comme l'en-tête l'a déjà perdu : « acfdc2b, 3 commits behind local ».
- **R2. Une ligne de santé passe en défaut en même temps que l'icône.** « ▲ Memory » s'affichait à côté d'un signe
  vert, parce que la ligne n'attendait pas la retenue de 120 s du constat. Une ligne prend ▲ ou ■ quand son constat
  est retenu : au même moment que l'icône, la ligne du menu et la notification. Avant, elle garde ● « Normal ».
- **R3. Les mots du détail d'une instance** (ce sont aussi ceux de `--status`) :
  - l'initiale en capitale, comme dans Health ;
  - « Simulated, answering » et « Real model, answering » pour « fake, ok » et « real model, ok » ;
  - « Queue 0 » comme dans Health, pour « queue 0/0 » ;
  - « From local at 7db54f8 » et « From a copy » pour « local @ 7db54f8 » et « copy » ;
  - le commit d'une instance d'essai, plutôt que « dev » seul.
- **R4. Le détail déplié** :
  - paires et boutons décalés jusqu'au texte du titre (environ 12 points) : sans filets, le retrait dit à quelle
    instance ils appartiennent ;
  - les boutons rangés par sens. Les adresses : Open in Browser · Copy Address · Copy Remote Address. Les
    fichiers : Show Log · Show Config ;
  - « Stop Instance… » aligné sur la première rangée.
- **R5. La barre d'outils qui rétrécit** cède d'abord Restart, le geste le plus rare, et garde Open Clairance,
  celui de tous les jours (`visibilityPriority`).
- **R6. Le serveur trop ancien** se dit sans le nom du binaire : « Not known: this version of Clairance does not
  say » ; au menu, « Not watched: this version of Clairance is too old ».

**Deuxième relecture** (captures 20 à 31). R1 à R6 sont faits (0eef3fa) :
- R1 : une seule lecture, celle du lanceur dans le dépôt, pour l'en-tête et pour la ligne Server ; le commit est lu
  pour toutes les instances.
- R2 : une ligne garde ce qu'elle disait en bon état tant que son constat n'est pas retenu, sauf si le lanceur démarre
  pendant la retenue, sans bon état connu.
- R3 : les deux nombres de la file s'écrivent « queue 4, 15 for a second reading ».
- R5 : à 480, Stop part dans « » » avec Restart (deux gestes rares, qui sont aussi au menu), et Open Clairance reste.

Deux ajouts du propriétaire, codés le même jour et relus :
- **une notification quand un compte se connecte** : titre « gabriel connected », corps « 1 page open, listening to
  124.350. », un clic ouvre la fenêtre. Rien pendant les deux minutes qui suivent un démarrage, le temps que les pages
  reviennent d'elles-mêmes, ni pour une page rouverte dans les 10 min, ni pour un compte Admin ;
- **dans les réglages, un interrupteur par sorte de notification**, tous allumés par défaut.

Retouches de ce tour :
- **N1.** La ligne Transcription d'une instance se coupe en deux : « Real model, answering · uses the GPU », puis
  une ligne « Queue » (« 4, 15 for a second reading »). Sinon, à 600 points, la valeur passe seule sous son
  libellé.
- **N2.** L'en-tête d'un binaire qui n'est pas dans la branche lit le jour du commit sans « -dirty », et finit
  comme la ligne Server : « Version of 20 November 2026 · 25d83ce-dirty · not in local ».
- **N3.** Les réglages des notifications, en deux sections :
  - « Notify when » et les sept défauts, avec le pied « A fault shows on the icon, in the menu and in the window
    either way. » ;
  - à part, « Notify when someone connects », avec le pied « Visits from Admin accounts are not notified. ».

  « owner » n'est pas un mot de l'écran : le rôle s'y appelle Admin.
- **N4.** « A test instance, a sidecar or a station link is left running » remplace « … or a helper … » : ce sont
  les mots du menu et de la fenêtre.

**Pas encore relu**, pas encore à l'image : le défaut avec sa cause longue et ses boutons du moment, la section
Permissions, le serveur muet (○), le menu allégé. Leurs captures envoient de vraies notifications : elles se feront
à un moment convenu avec le propriétaire.
