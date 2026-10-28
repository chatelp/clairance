# Fiche U — `go test -race ./...` vert sur une machine Linux neuve

*Écrite le 28/10/2026. Lis d'abord les règles communes de `README.md`. Crédit restant : environ 8 $ ; **la fiche est
courte, reste dans son périmètre**, et écris le rapport même si une étape n'est pas finie.*

## Pourquoi

Clairance doit s'installer ailleurs que sur la station de son propriétaire (bloc E du plan, `docs-fr/35-plan.md`).
Le premier geste d'une personne qui l'installe, ou d'une future intégration continue, c'est de lancer les tests. Sur
le Mac du propriétaire, `go test -race ./...` passe. Dans la session cloud de la fiche T (28/10, Linux, utilisateur
root), **trois tests échouaient, aussi sur `main` intact** :

- `TestAChoiceThatCannotBeWrittenChangesNothing` (`internal/auth/access_test.go`) : il attend qu'une écriture soit
  refusée, mais root écrit partout ;
- `TestAnHourOfStream` (`third_party/goxiph/mp3/live_test.go`) : trop long sous `-race` ;
- `TestASlowClientIsCutAndALongAnswerIsNot` (`cmd/server/serve_test.go`) : échoue une fois sur deux en suite
  complète, vert seul (sensible au temps).

## Les étapes (un commit chacune)

1. **Reproduire.** `go test -race ./...` deux fois de suite ; note chaque échec, son message, sa durée. Si d'autres
   tests échouent, ajoute-les à la liste et traite-les de la même façon. Ne change rien à ce qui passe.
2. **Pour chaque test fragile, la cause, puis le correctif le plus étroit**, sans affaiblir ce qu'il vérifie :
   - un test qui suppose de ne pas être root : qu'il le détecte (`os.Geteuid() == 0`) et le dise par `t.Skip` avec
     la raison, **ou** mieux, qu'il obtienne le refus d'une autre façon qui marche aussi pour root (un chemin sous un
     fichier, par exemple) ; préfère la seconde si elle existe ;
   - un test long sous `-race` : `testing.Short()` ne suffit pas (la commande n'a pas `-short`) ; regarde si la durée
     peut être réduite en gardant ce qu'il prouve (moins d'heures simulées), ou s'il doit être sauté sous la
     détection de course (`race` build tag : un fichier `race_on_test.go` / `race_off_test.go` qui pose une constante) ;
   - un test sensible au temps : mesure ce qu'il attend, élargis l'échéance avec une marge justifiée, ou remplace
     l'attente fixe par une attente d'un événement. Il doit passer dix fois de suite en suite complète
     (`go test -race -count=10 ./cmd/server/`).
3. **Vérifier** : `go test -race ./...` trois fois de suite, vert, et `go vet ./...`.

Rien d'autre : pas de nouvelle fonction, pas de changement du code hors des tests, sauf si un test révèle un vrai défaut
du code — dans ce cas, **arrête-toi**, décris-le dans le rapport, ne le corrige pas.

## Rapport

`docs-fr/fiches-cloud/U-rapport.md`, 40 lignes au plus : chaque test, sa cause mesurée, le correctif, et la preuve
(nombre de passages verts). Dis aussi ce que tu n'as pas pu reproduire.
