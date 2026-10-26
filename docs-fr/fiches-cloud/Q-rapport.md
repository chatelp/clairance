# Rapport de la fiche Q — un serveur qui porte sa page et dit ce qui lui manque (26/10)
**Branche : `claude/fiche-q-serveur-autonome-zytltf`** (la session cloud impose la sienne, pas `cloud/q-serveur-autonome`).
Partie de `f38f189`, le commit qui porte la fiche, où la session avait posé sa branche (`origin/main`, `e8f500b`, ne
la contient pas : 60 commits de retard) : 2 commits et ce rapport, rien sur `main`. Go 1.24.7, Node 22, en root. +1 467 / −157 lignes dans 27 fichiers, dont 772 de tests.

## Commits
- **Étape 1** `456e2a5` la page dans le binaire (18 fichiers, +495 −121). Paquet `www` (`www/page.go`) : `go:embed` de
  `index.html *.js *.css` et des 12 dossiers de la page, **pas de `*`** (un `www/data/` sur la machine qui compile
  partirait dans le binaire). Les tests Node sont incorporés avec leurs dossiers, `go:embed` ne sachant pas exclure un
  fichier (99 668 octets), et **jamais servis** ; le source Go n'est pas incorporé. `TestEveryFileOfThePageIsBuiltIn`
  nomme tout fichier de `www/` que la ligne `go:embed` oublie (vérifié : un dossier et un `.json` ajoutés le font
  échouer). Le gestionnaire lit un `fs.FS` (la page incorporée, ou `os.DirFS(static_dir)`) avec les mêmes en-têtes : ETag
  par contenu, `no-cache`, pas de date, GET/HEAD seulement, aucun fichier caché, pas de listage. `static_dir` reste,
  vérifié s'il est donné ; `www/` n'est plus exigé dans le dossier courant. Au démarrage « Page: embedded (commit …) » ou
  « Page: from <dossier> » ; `/health/detail` gagne `page` {`source`, `dir`, `commit`} ; `/vitals` gagne `page`
  (`embedded` ou `directory`, **jamais le chemin** : un chemin peut nommer un utilisateur). Le banc ne lie plus `www/`.
- **Étape 2** `12fb55e` le bilan au démarrage (13 fichiers, +972 −36). `internal/startcheck`, lancé avant le sidecar
  (pour qu'une absence soit nommée avant l'arrêt qu'il provoque). Une ligne par absence, `error` ou `warn`, avec ce qui en
  souffre et quoi faire ; la liste dans `/health/detail`, champ `missing` (`[]` si rien), figée au démarrage. Erreurs :
  ffmpeg **seulement si une source le demande** (toutes avec le décodeur ffmpeg, y compris celles ajoutées ensuite ; avec
  `decoder = "go"`, les sources qu'il ne lit pas, nommées — `audio.NeedsFFmpeg`, la règle même du décodeur) ;
  le programme et le script `.py` du sidecar si `command` est donné ; le modèle de `--model-en` s'il est un chemin
  local ; le dossier de données non inscriptible. Avertissements : `pdftotext` (les noms AIP se font sans les cartes :
  aucune route publiée) ; `--model-fr`, `--fr-detector` ; l'espace des bases face à ce qu'elles peuvent encore prendre
  avant que la rétention **en vigueur** (celle du panneau) efface un jour ; l'espace de l'archive face au plancher du
  sidecar (`--save-audio-min-free-gb`, 8 Go par défaut), mesuré où il le mesure ; le plan de l'aéroport. Un identifiant
  Hugging Face n'est pas jugé ; `models/…` l'est si `models/` existe là où tourne le serveur. Pour lire la rétention du
  panneau, `config.NewRuntime` est remonté d'`openStorage` dans `main` (il ne fait que lire un fichier et journaliser).
  `diskSpace` passe tel quel d'`internal/api` à `pkg/disk.Space`. La ligne ffmpeg de `startFrequencies` est remplacée.

## Les fichiers propres à la station (choix)
`/data/X` est servi d'abord depuis **`<storage.sqlite_base_path>/station/X`**, puis depuis la page (le `data/` de
`static_dir`, comme avant). Pas de réglage nouveau : le dossier de données est déjà propre à la station et hors du
dépôt. Un **sous-dossier à lui** plutôt que le dossier de données lui-même : la route de la page est publique, et rien
d'autre (bases, cache AIP) ne doit pouvoir être servi — testé, y compris `..` encodé. `fetch.py` écrit là par défaut
(lu dans la configuration qu'on lui passe). Sans le fichier, la carte ne dessine pas de plan : vérifié dans le code
(`response.ok ? … : null`) et dans Chromium (la 404 est la seule requête en échec, aucune erreur de script).

## Chiffres
- Binaire (`go build`, linux/amd64) : **21 242 098 → 23 998 811 octets** à l'étape 1 (+13 % ; page 2 602 316, tests
  99 668), **24 035 900** à l'étape 2.
- Démarrage jusqu'au premier 200 de `/api/v1/health` (exemple, faux tar1090 de `tools/startup-bench/fake.py`) :
  médianes de 10 : 844 ms avant, 814 ms après l'étape 1 ; 15 lancements alternés : 850 ms avant, 833 ms après l'étape 2.
  Le bilan tient dans la même milliseconde du journal. `compare.py` avant/après : seules diffèrent les lignes du bilan,
  « Page: … » et la ligne ffmpeg, avancée.

## Vérifié, et comment
- À chaque commit : `go build` (linux, windows, darwin/arm64), `go vet ./...`, `go test -race ./...`,
  `node tools/www-check/check.mjs` (OK, empreinte Tailwind comprise : aucun JS de `www/` touché),
  `python3 -m unittest discover tools/airport-layout`, `tools/startup-bench/run.sh` PASS sans `www/` à côté.
- **Deux échecs, identiques sans mes changements** : `internal/auth` `TestAChoiceThatCannotBeWrittenChangesNothing` (en
  root, un fichier en lecture seule s'écrit) ; goxiph `TestAnHourOfStream` (plus lent que sa borne sous `-race`, passe sans).
- Tests : la page incorporée par la vraie pile (types, ETag, 304, HEAD, 405, tests et `.go` refusés), les fichiers de la
  station (ordre, bases jamais servies), `page` dans la santé et les vitals ; un test par cas du bilan avec des chemins
  inexistants et un `PATH` vide ; `main()` lancé avec `PATH` vide : il nomme ce qui manque, démarre, sert sa page.
- Le vrai binaire depuis un dossier vide, exemple sans `static_dir` : `/`, `app.js`, `tailwind.css` en 200 ;
  `/health/detail` dit `embedded` et liste ici le plan de l'aéroport ; avec `PATH=/nonexistent` : ffmpeg (erreur),
  pdftotext, le plan. Chromium sans tête : 61 fichiers chargés, rien d'autre en échec ; 62 et aucune erreur avec le plan.

## Pas vérifié
Le Mac (darwin seulement compilé), Homebrew, la production, le lanceur Swift qui devra lire `page` et `missing`, un vrai
sidecar et ses modèles, un disque externe débranché, les vrais flux ; la page n'a été regardée qu'avec de fausses données.

## Doutes, et à regarder en priorité sur le Mac
1. **Une instance sans `static_dir` sert désormais la page de son binaire**, plus le `www/` de son dossier courant : une
   instance d'essai qui comptait dessus doit écrire `static_dir = "www"` pour voir les modifications au rechargement.
2. **Un `static_dir` qui n'existe pas est refusé au démarrage** (avant : 404 partout). La production actuelle recopie
   `runs/production/www` avant de démarrer : pas concernée.
3. **Passer la production sur la page incorporée** : retirer `static_dir` et la copie par `rsync` du script ; poser le
   plan de l'aéroport dans `<sqlite_base_path>/station/` (copie de `www/data/…`, ou `fetch.py`) — le bilan le signale sinon.
4. Un dossier ou un genre de fichier ajouté en haut de `www/` doit l'être à la ligne `go:embed` : le test le dit, à
   condition de lancer `go test ./...` avant de compiler la production.
5. `missing` est figé au démarrage : un outil installé depuis reste listé jusqu'à la relance (dit dans le code).
6. Fiche R, en parallèle, ajoute aussi des champs à `health_detail_handlers.go` : conflit textuel probable à la fusion.
