# `docs-fr/` — le dossier de chantier

Ce dossier est **la documentation de travail du fork `clairance`**, en français.
Ce n'est pas une traduction de `docs/` : `docs/` appartient au projet amont et reste
tel quel, y compris `LOCAL-STT.md`, qui nous sert de référence.

Convention du fork : **code, commentaires et messages de commit en anglais ;
documents de chantier et échanges avec le propriétaire de la station en français.**

| Fichier | Contenu |
|---|---|
| `00-mission.md` | ce qu'on veut faire et pourquoi, le périmètre, le critère de réussite |
| `01-station.md` | l'installation existante : machine, réseau, chaînes RF, groupes de fréquences |
| `02-co-atc-amont.md` | le projet amont, **vérifié dans le code** le 15/09 ; **historique** (revue du 20/10) |
| `03-transcription.md` | le cœur technique : bilinguisme, modèles candidats, moteurs, détection de voix ; **historique** (état de septembre, revue du 20/10) |
| `04-corpus.md` | les 4 642 enregistrements réels disponibles et le protocole d'évaluation |
| `05-decisions.md` | **décisions prises et questions ouvertes — la mémoire du chantier** |
| `06-catalogue.csv` | les fréquences mesurées : niveau, écart crête-médiane, langue, à transcrire ou non |
| `07-carte-openai.md` | où l'amont appelle OpenAI, avec quels formats — le plan du remplacement |
| `08-premiere-mesure.md` | premières mesures de transcription locale sur le corpus réel |
| `09-jeu-de-test.md` | construction du jeu d'évaluation et protocole d'annotation |
| `10-mesure-amorces.md` | ce que coûte l'amorce de transcription : dégénérescence ×7 |
| `11-demande-station.md` | ce qui a été demandé à l'agent de la station, et pourquoi |
| `12-concatenation.md` | concaténer les transmissions consécutives : mesure et verdict |
| `13-strategie-bilingue.md` | la stratégie bilingue : routage par langue, modèles candidats |
| `14-mesure-modeles-francais.md` | comparatif des Whisper francisés sur le corpus |
| `15-chaine-locale.md` | la chaîne locale de bout en bout : sidecar, segmenteur, contrat ; **historique** (état de septembre, revue du 20/10) |
| `16-grammaire-et-125933.md` | la grammaire de phraséologie, et l'identification de 125,933 |
| `17-appariement.md` | l'appariement des indicatifs contre l'ADS-B : méthode et témoin |
| `18-nuit-du-15.md` | la nuit de mesure : modèle anglais tranché, étagement de la TMA |
| `19-appariement-en-ligne.md` | la mise en service de l'appariement : point de greffe, réglages mesurés, limites |
| `20-captation-nuit.md` | **compte rendu de l'agent station** : captation nocturne 132-133, 4 h, 7 canaux |
| `21-nuit-132-133.md` | la nuit du 15 : 88 % de déclenchements sans parole, le peigne à 100 Hz |
| `22-correction-lexicale.md` | corriger le texte sans IA : mesuré, gain nul, **abandonné** |
| `23-voies-restantes.md` | **historique (16/09)** : l'état mesuré ce jour-là, tout ce qui avait été éliminé, le panel de relecture, la version courte à la fin ; pour l'état actuel, voir `33-pilotage.md` |
| `24-identification-amas.md` | **les sept fréquences de l'amas identifiées canal par canal** : Paris arrivée, Brest, cinq croisières |
| `25-atis-131025-au-gain.md` | l'ATIS de Saint-Cyr au gain : il ne sature pas, et ne porte pas de météo à 21 h (Q32) |
| `26-ce-que-co-atc-interprete.md` | ce que co-atc comprenait et affichait, par la voix et par l'ADS-B, vérifié dans le code — **historique, état au 23/09** (l'interface a été refaite depuis) |
| `27-augmentation-donnees.md` | **plan** : affiner le modèle anglais sur des corpus transcrits dégradés au bruit de la station (Q43) — étapes, juges, points d'arrêt |
| `28-gros-porteurs-par-canal.md` | les quatre canaux de `gros-porteurs` enregistrés séparément le 24/09 : deux sains, deux faibles qui battent et s'ouvrent sur du bruit (Q37, Q41) |
| `29-approches-reconnaissance.md` | **référence** : améliorer la transcription — le modèle de langage, la chaîne et la réception : ce qui est mesuré, adopté, écarté, et ce qui reste à tester — sans jargon |
| `30-association-regles-et-reglages.md` | **référence** : relier une transmission à un avion — chaque règle, son réglage dans co-atc (panneau, `runtime-settings.json`, `config.toml`), sa valeur par défaut, ce qu'elle rapporte |
| `31-vue-par.md` | conception et suivi de la vue PAR, le ciel de profil piste par piste (D67) — décisions, sources des données, questions ouvertes, étapes d'implémentation et leur vérification ; **historique, état au 29/09** |
| `32-audit-produit.md` | **journal du chantier produit** : l'étude du 09/10 (Q52 — mesure de l'écart avec l'amont, trois audits parallèles, consolidation et arbitrage), puis, dans sa section « État », chaque livraison datée depuis D71 (10/10), la plus récente en premier |
| `33-pilotage.md` | **pilotage** (ouvert le 12/10, **dernière mise à jour : 20/10**) : la stratégie en cinq lignes, les sessions parallèles et leur état, les jalons faits / en cours / à venir avec leurs dépendances, les décisions attendues du propriétaire, les risques — tenu à chaque livraison et décision ; « où en est-on ? » se répond ici |
| `34-lanceur.md` | **proposition** (24/10) : le lanceur de la barre des menus en poste de surveillance — gardien (santé, notifications, lancement au nom du lanceur), tableau de bord, poste de commande ; mission de l'agent dédié |
| `35-plan.md` | **le plan des étapes à venir** (26/10, avec le propriétaire) : les blocs — production fiable, serveur auto-contenu (Q60), contrat entre le lanceur et le serveur (Q61), accès à distance d'un proche (Q62), installable ailleurs, produit, crédit cloud — avec leurs étapes, leurs dépendances, leurs mesures de sortie et leur enchaînement |
| `35-plan-tableau.html` | **le tableau d'avancement du plan**, en blocs logiques, publié en artefact privé ; tenu synchrone avec `35-plan.md`, `05` et `33` à chaque étape (règle du propriétaire, 26/10) |
| `audit-2026-10-09/` | **rapports bruts** de l'audit du 09/10, copiés dans le dépôt le 10/10 pour que les sessions cloud les lisent : quatre rapports, `ux.md`, `backend.md`, `frontend-code.md` et `fonds-de-carte.md` |
| `fiches-cloud/` | **fiches autonomes pour les sessions cloud** (crédit de 250 $ jusqu'au 5/11) : règles communes, marche à suivre pour le propriétaire, une fiche par tâche ; les rapports des sessions y reviennent. Fiches A à M, **toutes intégrées**, rien en attente (20/10) |
| `fiches-labo/` | **fiches pour l'agent du laboratoire** (session « Whisper-lab », D63) : une tâche chacune, lancée par le propriétaire ; la première, la précision de l'appariement par niveau d'indices (12/10) |
| `design/` | **l'identité et le système de design** (ouvert le 12/10) : la mission de l'agent dédié (`MISSION.md`), son inventaire, ses principes, le système et ses jetons, les maquettes, l'identité (05), les avions (06), les esquisses (07), le design system (08), les écrans finaux (09) et le plan de migration (04), **terminée le 15/10** ; la session poursuit sur la branche `design-suite` |
| `CLAUDE-amont.md` | copie des instructions amont, conservées telles quelles |

> **Documents historiques** (revue du 20/10) : `02`, `03`, `15`, `23`, `26` et `31` décrivent l'état de
> septembre (15/09 à 29/09) ; leurs mesures datées restent exactes à leur date, mais l'interface, la
> chaîne et le dépôt ont changé depuis. Pour l'état d'aujourd'hui : `33-pilotage.md`, puis la section
> « État » de `32-audit-produit.md` et `05-decisions.md`.

> Le document 20 arrive numéroté 15 par l'agent de la station, qui ne pouvait pas
> savoir que `15-chaine-locale.md` existait déjà. Renuméroté ici, contenu intact.

> Le dépôt s'appelait `co-atc-local` jusqu'au 16 septembre (D20). `11-demande-station.md`
> et `20-captation-nuit.md` gardent l'ancien nom : c'est de la correspondance datée entre
> agents, et on ne réécrit pas une lettre après coup.

> Les dossiers audio du laboratoire `~/Dev/Aero/whisper-lab/`, hors dépôt, ont été rangés et
> renommés le 23/09 (D50) : `jeu-de-test/` est devenu `audio/2026-09-15-jeu-de-test-120/`,
> etc. ; le même jour, scripts, résultats et journaux sont passés dans `scripts/`,
> `resultats/` et `journaux/`. Les documents datés gardent les anciens chemins, pour la même raison. **La table de
> correspondance et l'index du laboratoire sont dans `whisper-lab/CORPUS.md`.**

`05-decisions.md` se tient à jour au fil de l'eau. C'est la seule consigne non
négociable de ce dossier.
