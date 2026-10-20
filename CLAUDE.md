# Clairance — instructions permanentes

> **Langue, avant tout le reste : tout ce qui s'adresse au propriétaire est en français**, y
> compris chaque courte note d'avancement entre deux commandes pendant un long travail. Seuls le
> code, ses commentaires et les messages de commit sont en anglais. Rappelé deux fois les 25 et
> 26/09 : c'est dans les notes intermédiaires que la dérive vers l'anglais se produit.

> **Disposition du dépôt.** Ce dépôt **est** un fork de `yegors/co-atc` : son historique
> est celui de l'amont, qui reste accessible via le remote `upstream`. Le travail se fait
> sur la branche `local`.
>
> Publié depuis le 20/09 : **https://github.com/chatelp/clairance**, public. La branche
> `local` pousse vers `origin/main` — `main` est ce que voit un visiteur, `local` est le
> nom de travail. Attention en lançant `gh` : sans `-R chatelp/clairance` il résout
> parfois sur le remote `upstream` et échoue en 404.
>
> **Le projet s'appelle Clairance depuis le 13/10 (D74)** ; il s'appelait `atc-scribe` du 16/09
> au 13/10 et `co-atc-local` avant. Le dossier local garde son ancien nom
> (`~/Dev/Aero/atc-scribe`) tant que les sessions et les arbres de travail y sont attachés ;
> le module Go et le binaire gardent `co-atc`, lignée du fork. Les documents datés d'avant le
> 13/10 gardent l'ancien nom : on ne réécrit pas une lettre après coup.
>
> - `docs/` appartient à l'amont — **on n'y touche pas**. `docs/LOCAL-STT.md` en particulier
>   est le plan de transcription locale écrit par l'auteur amont, et il nous sert de référence.
> - `docs-fr/` est **notre dossier de chantier**, en français. Voir `docs-fr/README.md`.
> - Ce fichier a remplacé le `CLAUDE.md` de l'amont, conservé à l'identique dans
>   `docs-fr/CLAUDE-amont.md`.

Tu travailles sur un **fork de [Co-ATC](https://github.com/yegors/co-atc)** adapté à une
station de réception aéronautique privée à Fontenay-le-Fleury (Yvelines, France).

**Lis `docs-fr/00-mission.md` en premier.** Les autres documents de `docs-fr/` sont la
référence : ils décrivent une installation qui existe et qui tourne, pas un projet
à imaginer.

## Ce que tu dois savoir avant de toucher à quoi que ce soit

1. **La station de réception est en production.** Elle alimente FlightAware et
   Flightradar24 en continu depuis des semaines, et son propriétaire l'écoute tous
   les jours. Tu peux lire tout ce que tu veux dessus ; **tu ne modifies sa
   configuration qu'après avoir prévenu et obtenu un accord explicite**, et jamais
   la chaîne ADS-B.
2. **Le développement se fait sur le Mac**, pas sur la station. La station est une
   source de données, pas un environnement de build.
3. **Ce qui est écrit dans `docs-fr/` a été vérifié sur la machine**, à la date
   inscrite dans chaque document (le dossier a commencé le 15 septembre 2026). Si tu
   constates un écart, c'est le document qui est périmé : corrige-le dans le même geste.
   Les comptes rendus datés restent tels qu'écrits ; on ne réécrit pas une lettre après coup.
4. **Co-ATC amont** : `docs-fr/02-co-atc-amont.md` a été vérifié dans ses sources le
   15/09. Ce qui n'y est pas relu reste à vérifier dans le code avant de t'appuyer dessus.
5. **La production ne se lance pas depuis ta session.** Pour macOS, un processus lancé
   depuis ton outil Bash appartient à l'application Claude : quand elle se met à jour ou se
   relance, il continue de tourner mais perd le réseau local (« no route to host » vers la
   station, sans autre signe). C'est arrivé du 16 au 18/10 : un jour et demi sans
   transcription. Laisse le propriétaire la lancer (lanceur de la barre des menus), ou
   passe par le système (`open`). Même règle pour ce qui doit passer la nuit.

## Langue

Le projet est destiné à un dépôt public : **code, commentaires, noms de variables,
messages de commit et README en anglais**. Les documents de `docs-fr/` et les échanges
avec le propriétaire sont **en français**.

**Tout ce qui s'adresse au propriétaire est en français, sans exception** : les réponses, mais
aussi les courtes notes d'avancement entre deux commandes pendant un long travail, et les
messages à l'agent de la station. Le 25/09, une nuit de code a fait glisser ces notes en anglais,
et il a dû le rappeler.

## Méthode

- **Mesurer avant de conclure.** C'est la culture de ce projet : chaque affirmation
  technique du dossier est adossée à un chiffre relevé. Tiens ce niveau. Si tu ne
  peux pas mesurer, dis que tu n'as pas mesuré.
- **Une décision structurante se discute avant d'être codée.** Elles sont listées
  dans `docs-fr/05-decisions.md` ; tiens ce fichier à jour, c'est la mémoire du chantier.
- **Pas de secret dans le dépôt.** Ni clé d'API, ni mot de passe Icecast, ni adresse
  exacte. La position de la station est déjà publique via FlightAware, mais la
  configuration d'exemple doit rester générique.
- **Le dépôt reste un fork, mais la rebasabilité n'a plus valeur de contrainte.**
  Mesuré le 16/09 (D19), remesuré le 09/10 (Q52) : l'amont est silencieux depuis le 3 mai
  2026 ; notre empreinte sur ses fichiers, faible au départ (1,7 % de `www/app.js`, 0,2 % de
  `internal/adsb/service.go`), valait 8,8 % et 9 % le 09/10. Depuis, l'interface a été
  refaite (`www/app.js` : 5 512 lignes amont, 1 086 aujourd'hui) et l'avance dépasse
  800 commits (835 le 20/10).
  **Modifie un fichier amont quand c'est la façon la plus simple d'écrire la chose** ;
  n'ajoute pas d'accesseur pour contourner une signature. **Depuis le 10/10 (D71, validée),
  Clairance est un produit à part** : on ne rebase plus, ce qui n'a pas de rôle ici se
  retire au lieu de dormir, l'interface est la nôtre, et **ce qui est contribuable reste
  identifiable — un correctif contribuable = un commit autonome**, qui ne dépend d'aucun
  de nos retraits (liste dans D71). L'audit et son plan : `docs-fr/32-audit-produit.md`.
  Règle du chantier : **ne rien casser de ce qui marche** (sept garde-fous, doc 32).
  En revanche, **garde identifiable ce qui est contribuable** : le sidecar de
  transcription, le correctif de rotation de base (Q26), l'authentification et l'état
  serveur doivent pouvoir partir en pull request tels quels.

## Commandes utiles

`station.lan` est un nom générique : l'adresse réelle de la station et son nom d'hôte sont
hors du dépôt depuis le 12/10 (D75), dans la mémoire privée de la session (`adresses-station-privees`)
et dans `configs/config.toml`, non suivi.

```bash
go build -o bin/co-atc ./cmd/server          # le serveur (rm bin/co-atc avant : jamais recopier un binaire en place)
go test ./...                                # les tests Go
node tools/www-check/check.mjs               # le filet de la page (store, scripts, tests Node de www/)
tools/tailwind/build.sh                      # la feuille Tailwind, après un changement de classes
ssh pierre@station.lan                       # la station, sans mot de passe
curl http://station.lan:8080/data/aircraft.json    # ADS-B temps reel
curl -H 'Host: station.lan' http://station.lan/radio/etat   # etat de la chaine VHF
```

Aucun sudo n'est requis sur la station pour `/opt/adsb`, les conteneurs et les
gabarits. Les modifications système (sysctl, systemd) demandent un mot de passe que
tu n'as pas — passe par le propriétaire.

## Le dossier `docs-fr/`

L'index et le rôle de chaque fichier sont dans `docs-fr/README.md`. Trois repères :

- **Commence par `docs-fr/00-mission.md`.**
- **« Où en est-on ? » se répond dans `docs-fr/33-pilotage.md`**, tenu à chaque livraison et
  décision ; ce qui a été fait, au jour le jour, dans les entrées « État » de
  `docs-fr/32-audit-produit.md`.
- **`docs-fr/05-decisions.md` se tient à jour au fil de l'eau** — décisions prises d'un
  côté, questions ouvertes de l'autre. C'est la mémoire du chantier et la seule consigne
  non négociable de ce dossier.
