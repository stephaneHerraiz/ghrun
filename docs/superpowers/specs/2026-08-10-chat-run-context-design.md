# ghrun — Chat : discuter d'un run avec Claude, log et workflow en contexte

Date : 2026-08-10
Statut : validé (brainstorming) — en attente de plan d'implémentation

## Objectif

Depuis l'écran **run detail**, la touche `c` ouvre une **discussion interactive**
avec le CLI `claude`, pré-chargée avec le contexte du run : son **log** et le
**fichier workflow tel qu'il était au commit du run**.

Différence avec la feature **Explain** (`e`) existante :

| | Explain (`e`) | Chat (`c`) |
|---|---|---|
| Interaction | one-shot, réponse affichée dans un écran ghrun | multi-tours, dans le CLI `claude` |
| Runs concernés | échec / timed-out uniquement | **tous** les runs |
| Contexte | log tronqué à 64 KiB, injecté dans le prompt | log **complet** + YAML du workflow, en fichiers |
| Outils | aucun | tous ceux de Claude Code (lecture du repo, git, `gh`…) |
| Mémorisation | oui (base RAG locale) | non |

Les deux cohabitent : `e` reste la réponse instantanée, `c` sert à creuser.

## Choix structurants

- **Passage de main au CLI `claude`** via `tea.ExecProcess` (bubbletea 1.3.10) :
  ghrun rend le terminal, `claude` s'ouvre en plein écran, et en quittant on
  retombe exactement sur l'écran run detail. Aucune UI de chat à écrire, et on
  hérite de tous les outils de Claude Code.
- **Contexte livré en fichiers, pas inline.** ghrun écrit `log.txt` et
  `workflow.yml` sur disque et donne leurs chemins absolus dans le prompt
  initial. Aucune troncature : Claude lit et grep le log lui-même, quelle que
  soit sa taille.
- **Démarrage dans le clone local du repo** quand il est trouvé, pour que
  Claude puisse lire le code, l'historique git et les autres workflows.
  Détection par racines de recherche configurables (`chat.cloneRoots`, défaut
  `~/dev`). Fallback : le dossier de contexte.
- **Les fichiers de contexte vivant hors du clone**, la commande passe
  `--add-dir` pour autoriser Claude Code à les lire.
- **Workflow au commit du run**, pas la version courante de la branche :
  `gh api repos/{o}/{r}/actions/runs/{id}` fournit `path` + `head_sha`, puis
  `contents/{path}?ref={head_sha}`. Le clone local peut être sur un autre
  commit ; le YAML fourni est celui qui a réellement tourné.
- **Question par défaut dans le premier message** : Claude travaille dès
  l'ouverture plutôt que d'accuser réception.

## Flux

```
run detail ── c ──▶ prepareChat (async)
                      1. gh api …/actions/runs/{id}     → path, head_sha
                      2. gh api …/contents/{path}?ref=… → workflow.yml
                      3. gh run view {id} --log[-failed] → log.txt
                      4. écriture dans le dossier de contexte
                      5. FindClone(cloneRoots, repo)     → cwd
                    ──▶ chatReadyMsg{cmd}
                    ──▶ tea.ExecProcess : claude prend le terminal
                    ──▶ (sortie de claude) chatDoneMsg → retour run detail
```

Commande construite :

```
claude --add-dir <dossier de contexte> "<prompt initial>"
```

`Dir` = clone local si trouvé, sinon le dossier de contexte.

## Dossier de contexte

`~/.cache/ghrun/chat/<owner>/<name>/<runID>/` (honore `XDG_CACHE_HOME`, comme
le cache repos existant), contenant :

- `log.txt` — le log du run,
- `workflow.yml` — le YAML du workflow au commit du run.

Réécrit à chaque appui sur `c`. Pas de purge automatique (hors scope) : le
dossier est supprimable à la main sans conséquence.

## Prompt initial

Une seule chaîne, construite par `chat.Prompt`. Forme, pour un run en échec :

```
GitHub Actions run #4211 of sxd-platform/beyond-core — failure
Workflow: Build & test (.github/workflows/ci.yaml)
Branch: feature/x · commit df53ccd · https://github.com/…/actions/runs/4211
Failed steps: build / npm ci; build / lint

Context files (read them before answering):
  /home/…/log.txt        failed-job log (gh run view --log-failed)
  /home/…/workflow.yml   the workflow file as it was at df53ccd

You are in a local clone of this repository (it may be on another commit).
For the full log of every job: gh run view 4211 --repo sxd-platform/beyond-core --log

Analyse the root cause of this failure, then propose a fix.
```

Variantes :

| Situation | Effet sur le prompt |
|---|---|
| Conclusion `failure` ou `timed_out` | log `--log-failed` ; question « analyse la cause racine, puis propose un correctif » |
| Toute autre conclusion | log `--log` complet ; question « résume ce qu'a fait ce run » |
| Aucune step en échec | ligne `Failed steps:` omise |
| Clone local non trouvé | ligne `You are in a local clone…` omise |
| YAML indisponible | `workflow.yml` omis des fichiers, une ligne signale que le workflow n'a pas pu être récupéré |
| Log indisponible (run en cours) | `log.txt` omis, une ligne signale que le run n'est pas terminé |

Prompt en anglais, cohérent avec le TUI et avec les prompts de `explain`.

## Découpage du code

### `internal/chat` (nouveau paquet)

Pur : aucune dépendance à bubbletea, entièrement testable sans TTY.

Deux types :

```go
// Context : ce que l'UI sait du run.
type Context struct {
    Repo         string   // "owner/name"
    RunID        int64
    RunNumber    int
    Workflow     string   // "Build & test"
    WorkflowPath string   // ".github/workflows/ci.yaml"
    Branch       string
    HeadSHA      string
    Status       string
    Conclusion   string
    FailedSteps  []string // "job / step"
    WebURL       string
}

// Session : le contexte matérialisé sur disque, prêt à lancer.
type Session struct {
    Context
    Dir          string // dossier de contexte
    LogFile      string // chemin absolu, "" si le log n'a pas pu être récupéré
    WorkflowFile string // chemin absolu, "" si le YAML n'a pas pu être récupéré
    CloneDir     string // clone local, "" si aucun n'a été trouvé
}
```

Fonctions :

- `Prepare(baseDir string, ctx Context, log, workflowYAML []byte) (Session, error)`
  — crée le dossier et écrit les contenus non vides ; un contenu vide n'est pas
  écrit et le champ correspondant de la `Session` reste `""`.
- `FindClone(roots []string, repo gh.RepoRef) (string, bool)` — teste
  `<root>/<name>` puis `<root>/<owner>/<name>`, retient le premier candidat
  contenant un `.git`. Les `~` en tête de racine sont résolus. L'appelant pose
  le résultat dans `Session.CloneDir`.
- `Prompt(s Session) string` — rend le prompt initial ; c'est `CloneDir`,
  `LogFile` et `WorkflowFile` qui pilotent les variantes du tableau ci-dessus.
- `Command(claudeCmd string, s Session) *exec.Cmd` — construit l'argv et
  positionne `Dir` = `s.CloneDir` s'il est non vide, sinon `s.Dir`.

`Context.FailedSteps` est rempli côté UI par le helper `failedSteps(detail)`
existant dans `explain.go`, qui devient partagé entre les deux features.

### `internal/gh`

- `RunWorkflowRef(repo RepoRef, id int64) (path, headSHA string, err error)` —
  `gh api repos/{o}/{r}/actions/runs/{id} --jq …`.
- `WorkflowFile(repo RepoRef, path, ref string) ([]byte, error)` — `gh api
  repos/{o}/{r}/contents/{path}?ref={ref} --jq .content` + décodage base64.
- **Refactor** : `WorkflowInputs` fait aujourd'hui ce fetch et ce décodage
  inline ; elle est réécrite au-dessus de `WorkflowFile` pour qu'il n'y ait
  plus qu'un seul chemin de décodage (le nettoyage `\n`/`\r` avant base64
  déménage dans `WorkflowFile`).

### `internal/ui`

- `keys.go` — `keyChat = "c"`.
- `rundetail.go` — `c` émet `chatRequestMsg{repo, id, detail}` quand la feature
  est disponible ; `chatAvailable` est estampillé par l'App sur `pushMsg`,
  comme `explainAvailable` aujourd'hui. Le hint `c chat` s'ajoute au footer de
  l'écran.
- `messages.go` — `prepareChatCmd(client, cfg, repo, id, detail)` : enchaîne
  les appels `gh`, écrit les fichiers, cherche le clone, et renvoie
  `chatReadyMsg{cmd *exec.Cmd}` ou `errMsg`.
- `app.go` — sur `chatReadyMsg` : `a.suspended = true` puis retour de
  `tea.ExecProcess(m.cmd, func(err error) tea.Msg { return chatDoneMsg{err} })`.
  **Les `tickMsg` sont ignorés tant que `suspended`** — sans ça le ticker
  continue de lancer des `gh run view` pendant que claude occupe le terminal.
  `chatDoneMsg` remet `suspended = false`, relance le ticker et rafraîchit
  l'écran ; une erreur non nulle part en `errMsg`.
- `footer()` — la ligne d'aide « Runs: » gagne `c chat`.

### `internal/config`

Nouveau bloc, indépendant de `explain` (le chat doit fonctionner même avec
`explain.enabled: false`) :

```yaml
chat:
  enabled: true
  claudeCmd: claude
  cloneRoots:
    - ~/dev
```

| Clé | Défaut | Description |
|---|---|---|
| `chat.enabled` | `true` | Active la touche `c` sur run detail. |
| `chat.claudeCmd` | `claude` | Binaire CLI lancé. |
| `chat.cloneRoots` | `[~/dev]` | Racines où chercher le clone local du repo. |

## Erreurs

| Cas | Comportement |
|---|---|
| `claude` absent du `PATH` | Erreur rouge en footer, rien n'est lancé |
| Clone local introuvable | Lancement quand même, `Dir` = dossier de contexte |
| YAML du workflow inaccessible (fork, fichier supprimé, run très ancien) | Lancement sans le YAML, signalé dans le prompt |
| Log indisponible (run `queued` / `in_progress`) | Lancement avec le workflow et les métadonnées, signalé dans le prompt |
| Ni log ni workflow récupérables | Erreur en footer, rien n'est lancé (il n'y aurait plus de contexte) |
| `claude` sort en erreur | Message en footer, retour normal sur run detail |

Comme partout dans ghrun, ces erreurs sont non bloquantes : elles s'affichent en
rouge dans le footer et s'effacent au bout de quelques secondes.

## Tests

- **`internal/chat`** : rendu du prompt sur les six variantes du tableau
  ci-dessus ; `FindClone` sur une arborescence temporaire (`<root>/<name>`,
  `<root>/<owner>/<name>`, dossier sans `.git`, racine inexistante, `~` en
  tête) ; `Prepare` écrit bien les fichiers et omet les contenus vides ;
  argv et `Dir` de `Command`.
- **`internal/gh`** : `RunWorkflowRef` et `WorkflowFile` avec le `Runner`
  factice déjà utilisé par les tests existants ; non-régression de
  `WorkflowInputs` après le refactor.
- **`internal/ui`** : `c` émet `chatRequestMsg` quand la feature est
  disponible et rien sinon ; l'App suspend le ticker sur `chatReadyMsg` et le
  relance sur `chatDoneMsg` ; une erreur de `chatDoneMsg` remonte en `errMsg`.

Pas de test de bout en bout du lancement réel de `claude` : `Command` renvoie
un `*exec.Cmd` inspecté sans être exécuté.

## Hors scope (YAGNI)

- Purge automatique de `~/.cache/ghrun/chat`.
- Reprise d'une session précédente (`claude --resume`).
- Déclenchement depuis la liste des runs ou depuis l'écran Explanation.
- Injection de l'explication déjà générée par `e` dans le contexte du chat.
- Champ de saisie dans ghrun pour taper sa question avant de lancer claude.
- Support d'un autre CLI que `claude`.
