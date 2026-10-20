# Transcription locale — le cœur technique

> **Document historique — revue du 20/10.** Analyse fondatrice du 15/09, avant toute mesure ;
> le corps du texte n'est pas réécrit. Les choix qu'elle laissait ouverts sont tranchés :
> moteur **mlx-whisper** (D10) ; modèle anglais `sfabriece/whisper-large-v3-atco2-asr-mlx` (D12) ;
> seconde lecture française par `bofenghuang/whisper-large-v3-french`, déclenchée par un mot repère
> **ou** par un détecteur `whisper-large-v3-turbo` à p(fr) ≥ 0,1 (Q50, D64) ; langue **fixée par
> fréquence** (la détection automatique se trompait de 16 à 28 %) ; détecteur de voix = le paquet
> `silero-vad` sous MLX, et non faster-whisper. Pour l'état courant : `29-approches-reconnaissance.md`,
> `30-association-regles-et-reglages.md` et `sidecar/README.md`.

## Le problème, en une phrase

Les modèles Whisper affinés pour le contrôle aérien sont **anglophones**. La bande de
cette station est **bilingue**, parfois sur la même fréquence à une minute d'intervalle.

## Pourquoi c'est vrai ici et pas partout

En France, le contrôle se fait **en français avec les pilotes francophones et en anglais
avec les autres**. Sur 129,525 (aéroclub de Chavenay) c'est du français presque
exclusivement. Sur 124,625 (Paris Contrôle DG) c'est de l'anglais presque exclusivement.
Sur 127,750 (Orly Départs) ou 118,700 (Orly Tour), les deux alternent au fil des avions.

Transcriptions réelles obtenues sur cette station le 15 septembre, modèle `small`
multilingue, détection de langue automatique :

```
132,500  20h08  en (0.68)  « Radar, bonjour, Qatari [...] heavy, flight level 380,
                             on track to Vestan »
132,500  20h08  sv (0.33)  « …380, radar heading 020 »        <- langue mal detectee
128,950  11h34  fr (0.87)  « Je crois que ca rentre, mais je rappelle que je peux
                             passer par la verticale... »
```

Deux enseignements : le bilinguisme est bien réel, et **la détection automatique de
langue se trompe** sur les transmissions courtes et bruitées (la deuxième ligne, de
l'anglais, est sortie en suédois avec 0,33 de confiance).

## Les modèles candidats

| Modèle | Base | Données | Résultat annoncé | Langue |
|---|---|---|---|---|
| [`jacktol/whisper-medium.en-fine-tuned-for-ATC`](https://huggingface.co/jacktol/whisper-medium.en-fine-tuned-for-ATC) | Whisper medium.en | ATCO2 + UWB-ATCC | WER **94,6 % → 15,1 %** | **anglais seulement** |
| [`jlvdoorn/WhisperATC`](https://github.com/jlvdoorn/WhisperATC) | plusieurs tailles | ATCO2, ATCOSIM | à vérifier | anglais |
| Whisper `large-v3-turbo` générique | — | — | référence multilingue | **multilingue** |

L'écart annoncé par jacktol est considérable — un modèle générique est quasi inutilisable
sur de l'ATC brut (94 % de WER, c'est du bruit), l'affinage le rend exploitable. Mais
appliqué à du français, un modèle `.en` produira au mieux une translittération absurde.

**Aucun modèle Whisper affiné pour l'ATC francophone n'a été trouvé.** C'est
précisément le vide que ce projet peut combler, et ce qui justifie un dépôt public.

## Les trois stratégies à évaluer

**A — Deux modèles, aiguillage par langue.** Un détecteur de langue en amont, puis
`jacktol` pour l'anglais et un Whisper multilingue pour le français. Le plus prometteur,
le plus lourd : deux modèles en mémoire, et l'aiguillage doit être fiable — or on vient
de voir qu'il ne l'est pas sur les transmissions courtes.

*Piste pour le fiabiliser :* la fréquence donne un **a priori** fort. 129,525 est
française, 124,625 est anglaise. Un a priori par fréquence, corrigé par la détection,
vaudra mieux que la détection seule. Et sur une transmission courte, allonger la fenêtre
d'analyse en concaténant la transmission précédente de la même fréquence peut aider.

**B — Un seul modèle multilingue générique**, `large-v3-turbo`. Simple, une seule
mémoire, pas d'aiguillage. Qualité probablement médiocre sur les indicatifs et les
chiffres — ce sont justement les mots qui comptent.

**C — Affiner soi-même un modèle multilingue sur de l'ATC français.** Le vrai apport
scientifique, et de loin le plus long. Le corpus de la station (`04-corpus.md`) est un
point de départ mais il n'est pas annoté. **À garder comme horizon, pas comme première
étape.**

**Commencer par mesurer A et B sur le corpus réel**, avant de choisir. Le protocole est
dans `04-corpus.md`.

## Le moteur d'exécution sur le M4

Le service tourne sur un Mac mini M4. Trois moteurs possibles, mesurés par des tiers en
2026 :

| Moteur | Note |
|---|---|
| **mlx-whisper** | **~2× plus rapide que whisper.cpp** sur Apple Silicon ([banc d'essai](https://notes.billmill.org/dev_blog/2026/01/updated_my_mlx_whisper_vs._whisper.cpp_benchmark.html) : 13,1 s contre 26,7 s sur le même extrait, en large-v3-turbo). Framework MLX d'Apple. **Candidat par défaut.** |
| whisper.cpp | portable, CoreML possible, la référence historique |
| faster-whisper | CTranslate2 ; **déjà installé et fonctionnel sur la station Fedora**, pratique pour comparer, mais pas optimisé Apple Silicon |

**Point de vigilance** : mlx-whisper doit pouvoir charger un modèle affiné. Il existe un
outil de conversion depuis un point de contrôle HuggingFace — **à vérifier sur le modèle
de jacktol avant de s'engager sur la stratégie A.** Si la conversion échoue ou dégrade
le modèle, tout l'arbitrage change.

## Le détecteur de voix, obligatoire

**La nuit, jusqu'à 98 % des déclenchements de squelch ne contiennent aucune parole**
(parasite secteur, voir `01-station.md`). En journée le plancher est de 18 à 34 %. Sans
filtre, la chaîne de transcription passera l'essentiel de son temps sur du bruit.

Le détecteur **Silero**, livré avec faster-whisper, tranche très proprement et **cent
fois plus vite** que le modèle complet. Calibration mesurée sur 180 fichiers tirés au
sort sur cette station :

| Fenêtre | Sans parole | Fraction parlée médiane |
|---|---|---|
| nuit 00-06 h | **98 %** | 0,00 |
| soir 18-22 h | 28 % | 0,81 |
| matin 07-09 h | 29 % | 0,81 |

La séparation est **bimodale** : dans un vrai échange, quatre cinquièmes du fichier sont
de la parole ; dans un déclenchement parasite, il n'y en a pas du tout. Il n'y a pas de
zone grise à arbitrer.

Réglage retenu : `VadOptions(threshold=0.5, min_speech_duration_ms=250,
min_silence_duration_ms=300)`, seuil de rejet à **moins de 0,25 s de parole**.

## Reconnaître un parasite plutôt que de le transcrire

Quand le détecteur ne trouve rien mais que le niveau est fort, le spectre dit pourquoi.
**Un peigne d'harmoniques espacées de 100 Hz** est la signature d'un parasite d'origine
secteur (redressement pleine onde du 50 Hz). Mesuré sur cette station, quatre fréquences
différentes, spectre moyen sur fenêtres de 4096 points à 8 kHz :

```
NUIT  133.000  pics : 400  500  600  699  801  1000  2373 Hz
NUIT  133.250  pics : 400  500  600  701  801  1002  2359 Hz
SOIR  132.500  pics : 451  463  516  525  553  629   643 Hz   <- parole
```

Le peigne ne commence qu'à 400 Hz parce que le passe-haut du canal coupe à 300.

> ⚠️ **Le peigne ne suffit pas comme critère de tri fichier par fichier** : il ne
> ressortait que sur 39 % des extraits de nuit. Il identifie la nature du parasite ;
> c'est le détecteur de voix qui trie.

## Ce qu'il faudra produire, et au bon format

Le remplacement doit rendre **ce que Co-ATC attend**, pas ce que Whisper produit
naturellement. Avant d'écrire une ligne de transcription, lire `prompts/` et `internal/`
en amont pour relever exactement :

- le format de sortie attendu (texte brut ? segments horodatés ? JSON structuré ?)
- ce que le post-traitement IA fait du texte, et s'il peut être remplacé par des règles
- comment les clairances sont extraites, et si ça survit à un texte français

Le vocabulaire aéronautique est très contraint — indicatifs, niveaux de vol, caps,
QNH, pistes. **Un post-traitement par règles peut rattraper beaucoup d'erreurs de
Whisper** sans appeler un modèle de langue : normaliser « trois cent quatre-vingts » en
FL380, reconnaître les indicatifs des compagnies qui desservent réellement CDG et Orly,
corriger les caps impossibles. À creuser avant de conclure qu'il faut un LLM local.
