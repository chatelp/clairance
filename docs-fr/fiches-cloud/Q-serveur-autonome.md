# Fiche Q — Un serveur qui porte sa page et dit ce qui lui manque

*Écrite le 26/10/2026 au soir. Plan : `docs-fr/35-plan.md`, bloc B (étapes B2 et B5), décision Q60 dans
`docs-fr/05-decisions.md`. Lis d'abord les règles communes de `README.md`.*

## Pourquoi

Le propriétaire veut un serveur **auto-contenu** : qu'il tourne sur un Mac ou une machine Linux sans
dépendre d'une copie de fichiers à côté de lui ni d'outils trouvés par hasard. Deux pannes du 26/10 le
montrent : `ffmpeg` introuvable dans le `PATH` du lanceur (la production démarrait sans audio, sans le
dire), et la page servie depuis une **copie figée** (`runs/production/www`, recopiée par `rsync` à
chaque livraison de la page), qui peut ne pas correspondre au binaire qui tourne.

## Ce qui existe (vérifie-le dans le code)

- La page est servie depuis le disque : `server.static_dir` (`internal/config/config.go`),
  `StaticFileHandler` (`internal/api/static.go`) : `Cache-Control: no-cache`, ETag = empreinte du
  contenu, GET et HEAD seulement, aucun fichier caché (fiche O).
- `www/` contient aussi des tests Node (`*.test.js`), `www/vendor/` (OpenLayers, Alpine, la feuille
  Tailwind construite par `tools/tailwind/build.sh`), et le filet `node tools/www-check/check.mjs`.
- `www/data/airport-layout.geojson` est **propre à chaque station**, généré par
  `tools/airport-layout/fetch.py`, **ignoré par git** : il n'est pas dans le dépôt.
- La recherche d'outils : `pkg/tool` (`tool.Find`), utilisée pour `ffmpeg` (en voie de retrait : le
  décodage passe en Go, `third_party/goxiph`), `pdftotext` (`internal/aip`) et l'interpréteur du sidecar
  (`internal/transcription/sidecar.go`).
- La santé : `/api/v1/health`, `/api/v1/health/detail` (propriétaire, réseau local),
  `internal/api/health_detail_handlers.go`.

## Les étapes (un commit chacune, sur `cloud/q-serveur-autonome`)

1. **La page dans le binaire** (B2). `go:embed` de `www/` (sans les `*.test.js`, ni rien que la page ne
   charge), dans un paquet à part. Le binaire sert **sa propre page** par défaut. `server.static_dir`, s'il
   est donné, la remplace par un dossier du disque (pour développer la page sans recompiler) : garde ce
   comportement, et dis-le au démarrage (« page: embedded (commit …) » ou « page: from <dossier> »). Les
   mêmes en-têtes qu'aujourd'hui (ETag par contenu, `no-cache`, méthodes, fichiers cachés refusés), tests
   compris : la pile de test doit pouvoir servir la page incorporée.
   **Les fichiers propres à la station** (aujourd'hui `www/data/airport-layout.geojson`) ne peuvent pas être
   incorporés : ils sont servis depuis le dossier de données (`storage.sqlite_base_path` ou un réglage
   dédié, à toi de proposer le plus simple), sous la même URL, et leur absence ne casse pas la page
   (vérifie ce que fait la carte sans eux). Décris le choix dans le rapport.
   `/api/v1/health/detail` et `/vitals` disent d'où vient la page.
2. **Le bilan au démarrage** (B5). Au démarrage, le serveur vérifie ce dont il dépend **selon sa
   configuration** et nomme chaque absence avec ce qui en souffre : un outil (`pdftotext` → « noms AIP
   non rafraîchissables » ; `ffmpeg` seulement si une source le demande encore) ; l'interpréteur et le
   script du sidecar si la transcription est active ; les modèles s'ils sont des chemins locaux (un
   identifiant Hugging Face se télécharge : ne pas le juger) ; l'espace libre du dossier des bases et du
   dossier d'archive audio (`--save-audio`), au regard des seuils existants ; le dossier de données
   inscriptible. Une ligne par point dans le journal, au niveau qui convient (une absence qui empêche
   l'audio ou la transcription est une erreur, pas une information), et la liste dans
   `/api/v1/health/detail` (champ `missing`, vide si tout est là), pour que le lanceur l'affiche. **Le
   serveur démarre quand même** : il dit ce qui manque, il ne refuse pas de tourner. Un test par cas,
   avec des chemins inexistants et un `PATH` vide.
3. **Le rapport**, `docs-fr/fiches-cloud/Q-rapport.md`, en français : le choix pour les fichiers propres à
   la station, la taille du binaire avant et après, le temps de démarrage avant et après
   (`tools/startup-bench` s'il existe), ce qui est vérifié et comment, ce qui ne l'est pas.

## Ce que tu ne fais pas

- Pas de lancement de compagnons par le serveur (B3) ni de contrat avec le lanceur (bloc C) : d'autres
  fiches.
- Ne retire pas `server.static_dir` : le propriétaire et la session du Mac s'en servent pour les
  instances d'essai.
- Ne touche pas à `docs/` (l'amont), ni au sidecar Python.

## Vérifier

`go build ./...`, `go vet ./...`, `go test ./...` ; `node tools/www-check/check.mjs` (le filet de la page
doit rester vert, y compris l'empreinte de la feuille Tailwind) ; le vrai binaire démarré sans
`static_dir` avec `configs/config.toml.example` : la page répond (`curl` sur `/`, un script, la feuille de
style), `/api/v1/health/detail` dit « embedded » et liste ce qui manque dans ta machine.
