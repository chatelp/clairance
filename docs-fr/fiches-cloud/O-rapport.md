# Rapport de la fiche O — l'ouverture publique derrière un mandataire (26/10)
**Branche : `claude/beautiful-fermat-5bd3ug`** (la session cloud impose la sienne). Partie de `main` (`e8f500b`) : 16 commits
et ce rapport, rien sur `main`. Go 1.24.7, en root. +1 732 / −36 lignes dans 27 fichiers, dont 1 204 de tests.

## Étape 1 — qui peut quoi, derrière le mandataire (figé dans `internal/api/testdata/access.txt`)
Comme Traefik transmet (depuis sa machine, `X-Forwarded-*`, Host public gardé ; co-atc sur `0.0.0.0`, mandataire déclaré),
tableau régénéré depuis le routeur : une route ajoutée sans exemple fait échouer le test.

| Routes | Inconnu | Viewer | Viewer, `audio_replay = "all"` | Propriétaire |
|---|---|---|---|---|
| La page (`/*`) en GET, HEAD | 200 | 200 | 200 | 200 |
| La page, toute autre méthode (y compris TRACE, CONNECT) | 405 (200 avant) | 405 | 405 | 405 |
| `/health`, `/vitals`, `/auth/status`, `/setup/status`, `/auth/logout` | 200 | 200 | 200 | 200 |
| `POST /auth/login` mauvais mot de passe ; `POST /setup`, un compte existant | 401 ; 409 | 401 ; 409 | 401 ; 409 | 401 ; 409 |
| `/health/detail` | 403 | 403 | 403 | 200 |
| 24 lectures (avions, fréquences, transcriptions, état, réglages lus, station, météo, AIP, référentiels) | 401 | 200 | 200 | 200 |
| Le direct (`/stream/{id}`, GET et HEAD) et les extraits (`/transcriptions/{id}/audio`) | 401 | 403 | passe | passe |
| `/ws` | 401 | ouvert depuis la page publique | idem | idem |
| 7 écritures (`PUT` access, label, receiver, server/settings, sources ; `DELETE` sources ; `POST` aip-names/refresh) | 401 | 403 | 403 | passe |
| `GET /signins` (ajoutée, étape 4) | 401 | 403 | 403 | 200 |

## Commits (chiffres : lignes ajoutées / retirées)
- `e305443` test AIP qui lisait le rappel en course, rouge sous `-race` sur `main` (+11 −4). **Étape 1** `4868b84` la matrice
  ci-dessus, figée (+228). **Étape 2** `89086ec` un test par interdit : routes verrouillées, écritures du viewer, audio,
  WebSocket et origines, `/setup`, `/health/detail`, cookie, limite, nom inconnu (+344), tous verts sur le code d'avant ;
  `9b82251` garde : le hachage leurre garde le coût d'un vrai (+30).
- **Étape 3**, un commit par défaut, chacun avec un test rouge avant : `3c03dc3` page en GET/HEAD seulement (+50 −6) ;
  `dac7e1a` aucun fichier caché (+60) ; `7f3bd46` `ReadHeaderTimeout` 10 s (+94 −7) ; `a26153a` `X-Forwarded-For` lu de
  droite à gauche (+72 −2) ; `94fcfe3` limite par /64 en IPv6 (+84 −3) ; `4072c81` le limiteur oublie (+51 −2) ;
  `4357440` `/setup` refusé par mandataire (+63 −4) ; `d52dcec` `/setup/status` sans chemin (+29 −4) ; `2faa6ba` pas de noms
  de comptes au viewer (+27) ; `0465fa0` message WebSocket borné à 64 Kio (+39).
- **Étape 4** `365c76e` la trace : chaque connexion réussie ou refusée (compte s'il existe, raison, adresse donnée par le
  mandataire, mandataire, agent, heure), en mémoire et dans `signins.jsonl` à côté des comptes (0600, deux fichiers de 1 Mo au
  plus, relu au démarrage, ignoré par git) ; `GET /api/v1/signins` au propriétaire seul ; un nom qui n'est pas un compte n'est
  écrit nulle part, ni dans le journal (+468 −10). **Étape 5** `6b5c800` le README : exemple Traefik générique (+88).

## Les défauts trouvés, leur gravité
1. **`POST /setup` à travers un mandataire** — *inconnu, prise de contrôle* : co-atc sur `127.0.0.1`, mandataire sur la même
   machine, pas encore de compte : le premier visiteur créait le compte propriétaire (mandataire déclaré) ou choisissait
   « sans connexion » pour tout Internet (non déclaré). Pas le cas du propriétaire (Traefik ailleurs, un compte existe).
2. **IPv6** — *inconnu, contournement de la limite* : une adresse neuve de son /64 par essai, et plus de limite.
3. **`X-Forwarded-For`** — *inconnu, selon le mandataire* : derrière deux mandataires (un CDN devant Traefik), tout le monde
   partageait l'adresse du CDN et 8 erreurs de n'importe qui bloquaient tout le monde ; un mandataire qui ajoute sa propre
   ligne laissait le client choisir son adresse. Derrière le seul Traefik, résultat inchangé.
4. **Pas de `ReadHeaderTimeout`** — *inconnu, épuisement* (atténué par le `readTimeout` de 60 s de Traefik) : coupé à 10,5 s
   sur le vrai binaire. Mesuré : `ReadTimeout` ne coupe pas un flux GET, `WriteTimeout` si (400 octets sur 2 000).
5. **Le limiteur ne vidait jamais sa mémoire** — *inconnu, lent* : 10 002 adresses gardées au lieu de 2 dans le test.
6. **Fichiers cachés servis**, **toute méthode servait la page**, **`/setup/status` donnait le chemin absolu de `users.json`**
   (le répertoire personnel) — *inconnu, faible*. 7. **Noms des comptes au viewer** ; **message WebSocket sans borne** — *viewer*.

Tenu : cookie `Secure` derrière HTTPS, `HttpOnly`, `SameSite=Strict`, `Path=/` ; nom inconnu et mauvais mot de passe identiques
(coût réel, 21 essais ×2 : 115/118 contre 120/116 ms médians, puis 250 ms chacun) ; corps bornés (1 à 8 Kio) ; `/vitals` ne localise rien.
## Constats sans correction (à trancher)
- **Sessions** : 720 h glissantes côté serveur, mais le cookie expire 30 jours après la connexion (il n'est jamais renouvelé) ;
  un jeton volé, lui, glisse sans fin. Raccourcir `session_ttl_hours` ? Au propriétaire. Un WebSocket ou un flux ouverts
  survivent à la déconnexion : couper l'accès, c'est retirer le compte et relancer (sessions en mémoire).
- Le viewer lit des adresses internes : `url` des fréquences (flux de la station), `/adsb/source`, chemins de `/server` et `/config`.
- Un viewer peut ouvrir 10 flux par fréquence (100 en tout) et en priver le propriétaire.
- Bloqué par la limite, la connexion répond 401 ; `login.js` attend 429 (« Too many attempts ») : message moins clair.
- `Role()` traite une session dont le compte n'existe plus comme propriétaire : inatteignable aujourd'hui (aucun retrait à
  chaud). `/vitals` dit à tous la version, le commit et la durée de marche (désactivable dans les réglages).

## Vérifié sur le vrai binaire, et non vérifié
- **Un vrai Traefik** (son téléchargement a été refusé par le garde-fou de la session) : l'exemple se lit, ses valeurs
  (`readTimeout` 60 s, `writeTimeout` 0, `idleTimeout` 180 s) sont celles de la documentation, leur effet mesuré sur le
  `net/http` de Go, pas sur Traefik. **Un vrai navigateur**, la station, la production ; aucune CSP proposée.
- Fait : le vrai binaire derrière un mandataire simulé (`curl` avec les en-têtes de Traefik) : 405, cookie, `stream_port` 0,
  403 du viewer, trace relue après relance, en-têtes lents coupés à 10,5 s ; `tools/startup-bench` passe. `go test -race ./...`
  vert sauf trois **déjà rouges sur `main`** : `TestAChoiceThatCannotBeWrittenChangesNothing` (root ; vert sous `nobody`),
  `TestAnHourOfStream` (goxiph, 87× le temps réel sous `-race`), `TestStopReachesGrandchildren` (instable, 2 sur 8 sur `main`).

## À régler par le propriétaire
Traefik : routeur HTTPS et certificat, `passHostHeader` vrai, limite sur `/api/v1/auth/login`, en-têtes, `writeTimeout` à 0,
pas de `forwardedHeaders.insecure`, le port 8000 seul. co-atc : `host = "0.0.0.0"`, `trusted_proxies = ["<adresse de Traefik
vue du Mac>"]` (une adresse, pas une plage), `audio_replay = "all"`, le compte de l'ami par `-add-user` (rôle `viewer`).

## À regarder en priorité sur le Mac
1. L'instance d'essai **à travers Traefik**, dans un navigateur et au téléphone : connexion, son (D2), WebSocket, rien dans le
   journal `Refused …`. 2. L'accès direct sur le réseau local et par Tailscale, inchangé. 3. `configs/signins.jsonl` créé en
   0600, ignoré par git ; `radio-ctl-sync` y apparaît à chaque connexion. 4. Le diff de `access.txt` à chaque changement de route.
