# 11 — La fenêtre du lanceur (étape 2)

*20/11/2026, session design, à la demande de la session principale (étape 2 de `docs-fr/34-lanceur.md`).
Le propriétaire, le 20/11 : « améliorer l'app pour qu'elle indique qui est connecté au serveur — et aussi ne pas
tout passer par le menu déroulant, il faudrait une vraie fenêtre », « en respectant la charte graphique si
possible (voir avec l'agent design) mais ça doit rester une app franchement macOS ». Proposée, à valider par le
propriétaire. Maquette approchée : `docs-fr/design/lanceur/` (`tools/design/lanceur.py`) ; la vraie fenêtre
prend les contrôles du système, que la maquette imite.*

## Le principe

Le même que pour le menu (`10-lanceur.md`) : une application Mac, avec les scènes, les contrôles, la police et
les couleurs du système, le clair et le sombre suivis. L'identité de Clairance tient en quatre choses, comme dans
le menu : **le signe et ses six états**, **les formes des voyants** (● ▲ ■ – ○ ◐, une forme et un mot, jamais une
couleur seule), **le ton** (ce qui se passe, depuis quand, ce qui marche encore ; jamais « Error ») et **les
nombres de la marque** (« 2 h 14 min », « 1 200 », fréquences à trois décimales). Les couleurs des voyants
sont celles du système (vert, orange, rouge), qui s'accordent au clair, au sombre et au contraste accru. Les
mots de la fenêtre sont ceux du menu : une même chose ne porte jamais deux noms.

## La fenêtre

- **Une seule fenêtre**, scène `Window` (macOS 13), titrée **« Dashboard »**. Taille par défaut 560 × 680
  points, minimum 480 × 420, redimensionnable ; macOS retient son cadre. Fermer la fenêtre (⌘W) ne quitte rien :
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
  sont vrais et la liste vide ; « Not known… » dès que l'un est faux.

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
