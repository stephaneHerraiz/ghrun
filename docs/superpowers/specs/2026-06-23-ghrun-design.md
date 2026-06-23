# ghrun — CLI TUI interactif pour GitHub Actions

**Date:** 2026-06-23
**Status:** Design validé, prêt pour le plan d'implémentation

## Objectif

Un CLI interactif (TUI) pour **lancer** et **suivre** des workflows GitHub Actions
sur plusieurs repositories, en s'appuyant sur le CLI `gh` déjà authentifié.
Les deux usages (lancement manuel de `workflow_dispatch` et monitoring live des
runs) ont un poids égal.

Binaire : `ghrun`. Module Go : `github.com/stephaneHerraiz/ghrun`.

## Choix structurants

| Décision | Choix retenu |
|---|---|
| Usage | Lancer **et** suivre, à parts égales |
| Source des repos | Hybride : favoris en config + refresh/cache depuis `gh` |
| Stack | Go + Bubbletea (TUI riche, live refresh) |
| Accès GitHub | Lib officielle `go-gh` (`gh.Exec` + client REST), réutilise l'auth `gh` |
| Monitoring | Liste runs live + drill-down jobs/steps + logs + actions (rerun/cancel/web) |
| Lancement | Inputs **parsés du YAML** → formulaire dynamique |
| Écran d'accueil | **Dashboard multi-repos hybride** (agrégation + sert de sélecteur filtrable) |

## Navigation & écrans

Un seul programme Bubbletea avec une **pile d'écrans** (push/pop). Convention :
**majuscules = navigation globale (dispo partout), minuscules = actions contextuelles.**

En-tête : fil d'Ariane (`owner/repo › Workflows › launch: deploy.yml`).
Footer : raccourcis globaux toujours visibles + zone d'erreur.

### Carte des écrans

```
Démarrage : vérif `gh auth status` → KO = message `gh auth login` + sortie

[ACCUEIL] Dashboard multi-repos (live)
   Lignes = favoris : REPO · DERNIER RUN (workflow·branche) · ÉTAT · NB ACTIFS
   Refresh live (fan-out parallèle sur les favoris)
   `/` filtrer (= sélecteur)   `Enter` entrer dans le repo   `R` gérer favoris
        ↓ (Enter sur un repo)
   ── dans un repo ──────────────────────────────────────────────
   [W] Workflows            [U] Runs (live)
        ↓ (Enter)                ↓ (Enter)
   [LANCEMENT]              [DÉTAIL RUN] (live)
   choix branche               jobs + steps (✓/✗/●/⋯), durées
        ↓                       actions : l logs · r rerun · f rerun-failed
   [FORMULAIRE inputs]                    x cancel · o web
   (généré du YAML)             ↓ (l)
        ↓ (submit)          [LOGS] viewport scrollable (--log / --log-failed)
   dispatch → bascule auto
   sur le DÉTAIL du run lancé
```

### Raccourcis

**Navigation globale (depuis n'importe quel écran) :**
- `W` → liste Workflows du repo courant
- `U` → liste rUns du repo courant
- `R` → accueil/sélection Repo (changer de repo sans quitter)
- `Esc` → recule d'un cran dans la pile
- `?` → aide · `q` / `Ctrl-C` → quitter

**Actions contextuelles (minuscules) :**
- Listes : `/` filtrer · `Enter` ouvrir/drill-down
- Liste runs & détail run : `r` rerun · `f` rerun-failed · `x` cancel · `o` ouvrir web · `l` logs
- `g` refresh forcé (le live tourne déjà automatiquement)

### Comportements clés
- **Après un dispatch réussi**, bascule directe sur le détail du run déclenché
  (lien naturel entre « lancer » et « suivre »).
- **Live refresh** : s'arrête (ou ralentit ~15s) quand aucun run actif ; ~3-5s quand un run tourne.
- **Footer** : raccourcis contextuels + erreurs `gh` en rouge (non bloquantes).

## Structure du code

```
ghrun/
├── cmd/ghrun/main.go         # wiring : config + client gh → lance Bubbletea
├── internal/config/          # YAML ~/.config/ghrun/config.yaml + cache ~/.cache/ghrun/repos.json
│     Config{DefaultOrg, RefreshIntervalSeconds, RunListLimit, Favorites []RepoRef}
├── internal/gh/              # wrapper go-gh — interface Client (mockable) + impl réelle
│     types : RepoRef, Workflow, Input{Name,Type,Default,Required,Options}, Run, Job, Step
│     méthodes : AuthStatus, ListOrgRepos, ListWorkflows, WorkflowInputs,
│                ListBranches, DispatchWorkflow, ListRuns, GetRun,
│                RunLogs, Rerun, Cancel, OpenWeb
└── internal/ui/              # un fichier par écran (sous-modèles Bubbletea)
      root.go        # pile de nav, deps partagées, keymap globale, breadcrumb + footer
      dashboard.go   # accueil multi-repos (live, fan-out parallèle)
      workflows.go   # liste des workflows du repo
      launch.go      # choix branche + formulaire d'inputs dynamique
      runs.go        # liste des runs (live)
      rundetail.go   # jobs/steps (live)
      logs.go        # viewport scrollable
      keys.go styles.go  # lipgloss : icônes de statut, couleurs, keymap partagée
```

**Principes**
- Chaque écran est un sous-modèle isolé (`Update`/`View`) ; `root` gère la pile et le routage par messages.
- **Tous les appels `gh` sont async** via `tea.Cmd` → message résultat/erreur. L'UI ne bloque jamais.
- `Client` est une **interface** : impl réelle via `go-gh`, tests via faux *execer* renvoyant des fixtures JSON.

**Dépendances :** `bubbletea`, `bubbles` (list/viewport/textinput), `lipgloss`, `go-gh/v2`, `yaml.v3`.

## Mapping fonctions → `gh`

Via `go-gh` (`gh.Exec` + `--json`, client REST pour contenu de fichier & branches) :

| Fonction | Commande |
|---|---|
| Auth (démarrage) | `gh auth status` → KO = sortie avec message `gh auth login` |
| Refresh repos (`R`) | `gh repo list <org> --json nameWithOwner --limit 200` → cache |
| Workflows d'un repo | `gh workflow list -R o/r --json name,id,path,state` |
| Inputs d'un workflow | REST `repos/{o}/{r}/contents/{path}` → base64 → YAML `on.workflow_dispatch.inputs` |
| Branches | `gh api repos/{o}/{r}/branches --paginate --jq '.[].name'` |
| **Dispatch** | `gh workflow run {id} -R o/r --ref {branch} -f k=v …` |
| Runs (liste, live) | `gh run list -R o/r --limit N --json databaseId,number,workflowName,displayTitle,status,conclusion,headBranch,event,createdAt,startedAt,actor` |
| Détail run (live) | `gh run view {id} -R o/r --json status,conclusion,jobs` (jobs → steps) |
| Logs | `gh run view {id} -R o/r --log` / `--log-failed` |
| rerun / rerun-failed / cancel | `gh run rerun {id}` / `--failed` / `gh run cancel {id}` |
| Ouvrir web (`o`) | `gh run view {id} -R o/r --web` |

## Cas délicats traités explicitement

1. **Retrouver le run après dispatch.** `gh workflow run` ne renvoie pas l'ID du run créé.
   On poll `gh run list --workflow {id} --limit 5` juste après le dispatch et on prend le run
   le plus récent **créé après** l'instant du dispatch (avec quelques retries — le run apparaît
   avec un léger délai). Une fois trouvé → bascule auto sur son détail.
2. **Workflow sans déclenchement manuel.** Les inputs sont parsés **à la sélection** (pas en masse
   au chargement). Si pas de `workflow_dispatch` : message clair « ce workflow n'a pas de
   déclenchement manuel » au lieu de planter.

## Refresh live

`tea.Tick(interval)` → commande de rafraîchissement. Le dashboard rafraîchit les favoris
**en parallèle** (fan-out goroutines dans la `Cmd`, résultats agrégés). Le polling s'arrête
(ou ralentit ~15s) quand aucun run n'est actif ; ~3-5s quand un run tourne.

## Gestion d'erreurs

- Chaque appel `gh` → `err` mappée en `ghErrMsg`, affichée **en rouge dans le footer**, **non bloquante**.
- Exception : échec d'auth au démarrage = **fatal** avec message `gh auth login`.
- Erreurs réseau / filtre DPI corpo : remontées **verbatim** (pas de masquage).
- États vides gérés : aucun favori → invite à en ajouter ; aucun workflow dispatchable ; aucun run.

## Tests

- `internal/gh` : *execer* mocké renvoyant des fixtures JSON → on teste le **parsing**
  (workflows, inputs YAML, runs, jobs), en table-driven.
- `internal/config` : round-trip load/save, valeurs par défaut, fichier absent.
- `internal/ui` : transitions `Update` sur messages clavier (`W` push workflows, `Esc` pop,
  formulaire qui rejette un input `required` vide) — via `teatest` ou appels directs à `Update`.

## Config — `~/.config/ghrun/config.yaml`

```yaml
defaultOrg: stephaneHerraiz       # compte/org pour le refresh `R` (gh repo list)
refreshIntervalSeconds: 4
runListLimit: 30
favorites:
  - stephaneHerraiz/ghrun
  - stephaneHerraiz/<autre-repo>
```

Cache repos : `~/.cache/ghrun/repos.json` (liste rafraîchie depuis `gh repo list`).

## Hors scope (YAGNI)

- Édition de workflows / secrets.
- Gestion des artefacts / téléchargement.
- Multi-host GitHub Enterprise (on cible github.com via l'auth `gh` existante).
- Notifications système.
