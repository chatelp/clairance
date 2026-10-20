# La chaîne locale tourne — première transcription sans clé d'API

> **Document historique — revue du 20/10.** Compte rendu du 15/09 ; le corps du texte n'est pas
> réécrit. Depuis : le processeur OpenAI et `processor.go` sont retirés (91cf595), `LocalProcessor`
> est le seul moteur ; le démultiplexage stéréo n'a jamais été fait et n'a plus d'objet, la station
> diffusant un flux par fréquence (D57, D64) ; la grammaire de phraséologie est faite (doc 16).

*15 septembre 2026. Mesures : `runs/etape4-local.log`.*

## Ce qui est en place

```
audio.lan/aero.mp3  ──►  ffmpeg  ──►  segmenteur Go  ──►  service annexe  ──►  SQLite
 flux de la station      PCM 24k      au squelch         mlx-whisper local     + carte
```

- **`sidecar/`** — le service HTTP que `docs/LOCAL-STT.md` de l'amont spécifie et n'avait
  jamais écrit. Portier VAD Silero, aiguillage de langue avec chemin de refus.
- **`internal/transcription/local.go`** — `LocalProcessor`, derrière la même interface
  `ProcessorInterface` à deux méthodes que le processeur OpenAI. Rien en aval ne sait
  quel moteur a tourné.
- **`internal/transcription/sink.go`** — le stockage et la diffusion, communs aux deux
  moteurs. Aucun fichier amont n'a été déplacé ; `processor.go` ne gagne que quatre lignes
  d'aiguillage.
- **`backend = "local"`** dans `[transcription]`, plus une section `[transcription.local]`.

**Aucune clé d'API n'est renseignée.** C'est le critère de réussite de `00-mission.md`,
pour la partie transcription.

## Le segmenteur : pourquoi c'est facile ici

L'API temps réel d'OpenAI recevait un flux continu et disait elle-même où la parole
commençait et finissait. Un modèle local ne le fait pas : il faut découper.

Mesuré sur le flux de la station : **entre deux transmissions, le signal est à −90 dBFS
exactement** — du silence numérique, pas un plancher de bruit. RTLSDR-Airband ferme le
squelch et n'émet rien. Le découpage au squelch est donc sans ambiguïté, là où il serait
délicat sur une source type LiveATC. Un garde de durée maximale borne quand même le cas
d'une source bruyante.

## Ce que ça produit

Neuf transmissions en 90 secondes, sur le groupe `orly-approche` :

```
air france one zero eight one good good day jobair zero five two four     3,5s  ×7,7
speed one five zero knots jet two two nine                                3,3s  ×8,8
tower alfa four two eight romeo can we vacate via runway                  4,6s  ×10,7
cleared to land runway two five delta foxtrot delta oscar kilo yankee…    7,9s  ×12,5
to the right cleared to land i say five four six                          3,2s  ×8,7
```

De **×1,2 à ×12,5 le temps réel**. `runway two five` et `runway two seven` existent
vraiment à Orly et à De Gaulle.

## La mesure qui n'a besoin d'aucune annotation

C'est le point important. Depuis le début, l'absence de vérité terrain bloque toute mesure
de justesse. Or **Co-ATC en fabrique une en continu** : si la transcription dit un
indicatif et que cet avion est réellement dans le ciel à cette seconde-là, ce n'est pas
une coïncidence.

Premier essai, extracteur volontairement naïf — toute suite d'au moins trois chiffres
épelés, comparée aux 77 avions actifs :

| Entendu | Reconstitué | Dans le ciel ? |
|---|---|---|
| « i say **five four six** » | 546 | **ICE546 — actif** |
| « knots jet **two two nine** » | 229 | **MEA229 — actif** |
| « air france **one zero eight one** » | 1081 | non — aucun AFR1081 |
| « that squish **one two one** decimal **zero five five** » | 121 / 055 | c'est une **fréquence**, pas un indicatif |
| « up on **one two two zero** » | 1220 | idem |

**Deux appariements réels sur dix-huit suites extraites.** Mais le chiffre ne veut pas dire
grand-chose : l'extracteur ramasse les fréquences, les altitudes et les caps comme s'ils
étaient des indicatifs. `121 decimal 055` est un transfert de fréquence, pas un avion.

**C'est exactement le travail que décrit Q6** : une grammaire de phraséologie qui
distingue un indicatif d'une fréquence, d'un niveau de vol, d'un cap et d'un QNH. Le
prompt de post-traitement de l'amont fait ça avec un modèle de langue ; on veut le faire
par règles.

Une fois cette grammaire écrite, **le taux d'appariement devient une mesure de justesse
continue, gratuite, sur du volume réel.** Ce sera le premier chiffre de qualité du projet
qui ne demande pas d'annotation.

## Le démultiplexage stéréo — l'attribution par fréquence sans toucher à la station

Le flux `aero.mp3` mélange 5 canaux, chacun à une position stéréo distincte. Mesuré sur
180 s captées depuis le Mac, la loi de panoramique de RTLSDR-Airband compresse les valeurs
déclarées d'un facteur **0,66** : `observé = 0,66 × déclaré`, sans décalage. Une fois
ajustée, **90 % des trames de parole tombent à moins de 0,05 d'une position connue**.

| Fréquence | Langue | Position | Parole | Part |
|---|---|---|---|---|
| **125,825 De Gaulle Approche** | anglais | +0,20 | 71,0 s | 59,5 % |
| 125,933 non identifiée | ? | +0,40 | 14,0 s | 11,7 % |
| **124,625 Paris Contrôle DG** | anglais | 0,00 | 13,2 s | 11,1 % |
| **124,350 Approche CDG** | anglais | −0,20 | 9,0 s | 7,5 % |
| 123,875 Orly Approche | mixte | −0,40 | 0,7 s | 0,5 % |
| ambigu — recouvrements | | | 11,4 s | 9,6 % |

Squelch ouvert **66 % du temps**, et **78 % de la parole est sur des fréquences
anglophones**. C'est de l'**attribution**, pas de la séparation : si deux canaux parlent
en même temps le mélange contient les deux, ce qui est probablement une bonne part des
9,6 % d'ambigus.

**Conséquence** : la captation par canal demandée à la station (`11-demande-station.md`)
reste utile pour un corpus propre et pour régler le gain de l'ATIS de Saint-Cyr, mais elle
**n'est plus bloquante** pour transcrire l'anglais d'approche. Le matériel est déjà à
l'antenne et exploitable.

## Un bug amont trouvé au passage

Une erreur de syntaxe TOML dans `configs/config.toml` est signalée par Co-ATC comme
« config file not found in any of the expected locations ». La boucle de
`LoadWithFallbackAndPath` (`internal/config/config.go:331-350`) écrase la vraie erreur de
chargement par l'erreur « fichier absent » du chemin suivant. Candidat à une contribution
amont : deux lignes.

## Suite

1. **La grammaire de phraséologie** et le taux d'appariement ADS-B — la première mesure de
   justesse sans annotation.
2. **L'attribution par balance** branchée dans le segmenteur, pour étiqueter chaque
   transmission de sa fréquence.
3. Annoter les 120 transmissions, qui reste la seule mesure de justesse absolue.
