# ghrun

**ghrun** est un CLI interactif (TUI) pour **lancer** et **suivre** des workflows
GitHub Actions sur plusieurs dépôts, sans quitter le terminal. Il s'appuie sur le
CLI `gh` déjà authentifié — aucune configuration de token à gérer.

Écrit en Go avec [Bubbletea](https://github.com/charmbracelet/bubbletea).

```
ghrun › owner/repo › Workflows › launch: deploy.yml
```

---

## Sommaire

- [Aperçu](#aperçu)
- [Prérequis](#prérequis)
- [Installation](#installation)
- [Premier lancement](#premier-lancement)
- [Les écrans](#les-écrans)
- [Raccourcis clavier](#raccourcis-clavier)
- [Fonctionnalités](#fonctionnalités)
- [Configuration](#configuration)
- [Comment ghrun parle à GitHub](#comment-ghrun-parle-à-github)
- [Hors périmètre](#hors-périmètre)

---

## Aperçu

Deux usages à parts égales :

- **Lancer** un workflow `workflow_dispatch` : ghrun lit les `inputs` directement
  dans le YAML du workflow et génère un **formulaire dynamique** (texte, nombre,
  booléen, liste de choix), puis dispatche sur la branche choisie. Après le
  lancement, il bascule **automatiquement** sur le détail du run créé.
- **Suivre** les runs en **live** : un tableau de bord multi-dépôts agrège l'état
  de tes dépôts favoris, avec drill-down jobs/étapes, logs scrollables et actions
  (rerun, rerun-failed, cancel, ouvrir dans le navigateur).

Toute la navigation se fait dans une **pile d'écrans** (push/pop). Convention :
**majuscules = navigation globale** (disponible partout), **minuscules = actions
contextuelles**.

---

## Prérequis

- **[`gh`](https://cli.github.com/) authentifié** : lance `gh auth login` au
  préalable. ghrun vérifie `gh auth status` au démarrage et s'arrête avec un
  message explicite si l'auth manque. Scopes nécessaires : `repo` et `workflow`
  (ainsi que `read:org` pour lister les organisations).
- **Go 1.25+** uniquement si tu installes/compiles depuis les sources.

---

## Installation

Via `go install` :

```bash
go install github.com/stephaneHerraiz/ghrun/cmd/ghrun@latest
```

Ou depuis les sources :

```bash
git clone https://github.com/stephaneHerraiz/ghrun.git
cd ghrun
go build -o ghrun ./cmd/ghrun
./ghrun
```

---

## Premier lancement

1. ghrun vérifie que `gh` est authentifié.
2. Au tout premier lancement, un fichier de configuration vide est créé dans
   `~/.config/ghrun/config.yaml`.
3. Comme aucune organisation par défaut n'est définie, ghrun affiche un
   **sélecteur d'organisation** listant ton compte personnel puis les
   organisations dont tu es membre. Le choix est **persisté** dans la config.
4. Tu arrives ensuite sur le **tableau de bord**.

---

## Les écrans

| Écran | Rôle |
|---|---|
| **Sélecteur d'organisation** | Premier lancement : choisir le compte/organisation par défaut. |
| **Tableau de bord** (accueil) | Liste hybride : favoris (avec statut live) + dépôts de l'organisation. Sert aussi de sélecteur de dépôt filtrable. |
| **Workflows** | Liste des workflows d'un dépôt. `Enter` charge les `inputs` et ouvre le lancement. |
| **Lancement** | Choix de la branche (`ref`) puis formulaire d'`inputs` dynamique → dispatch. |
| **Runs** | Liste live des runs du dépôt courant. |
| **Détail run** | Jobs et étapes avec icônes de statut, rafraîchi en live. |
| **Logs** | Viewport scrollable des logs (`--log` ou `--log-failed`). |

Chaque écran affiche un **fil d'Ariane** en en-tête et un **pied de page** avec les
raccourcis et une zone d'erreur (les erreurs `gh` s'affichent en rouge, sans
bloquer l'UI).

---

## Raccourcis clavier

### Navigation globale (depuis n'importe quel écran)

| Touche | Action |
|---|---|
| `W` | Workflows du dépôt courant |
| `U` | Runs du dépôt courant |
| `R` | Retour à l'accueil / sélection de dépôt |
| `Esc` | Reculer d'un cran dans la pile |
| `?` | Afficher / masquer l'aide |
| `q` · `Ctrl-C` | Quitter |

> `W` et `U` ne font effet que lorsqu'un dépôt est sélectionné.

### Tableau de bord (accueil)

| Touche | Action |
|---|---|
| `↑`/`↓` · `k`/`j` · molette | Se déplacer dans la liste |
| `Enter` | Entrer dans le dépôt (ouvre ses runs) |
| `f` | **(Dé)favoriser** le dépôt en surbrillance (persisté en config) |
| `g` | Rafraîchir (favoris + dépôts de l'organisation) |
| `/` | Filtrer la liste (puis taper ; `Enter` entre dans le dépôt, `Esc` annule le filtre) |

### Workflows

| Touche | Action |
|---|---|
| `↑`/`↓` · `k`/`j` · molette | Se déplacer |
| `Enter` | Charger les inputs et configurer le lancement |

### Lancement — choix de la branche

| Touche | Action |
|---|---|
| `↑`/`↓` · `k`/`j` · molette | Choisir la branche (`main`/`master` présélectionnée) |
| `Enter` | Continuer vers le formulaire d'inputs |

### Lancement — formulaire d'inputs

| Touche | Action |
|---|---|
| `↑`/`↓` | Changer de champ |
| `←`/`→` | Modifier un choix ou un booléen |
| *(saisie)* | Taper directement dans un champ texte/nombre |
| `Enter` | **Lancer** le workflow (`Ctrl-S` fonctionne aussi, en repli) |

### Runs

| Touche | Action |
|---|---|
| `↑`/`↓` · `k`/`j` · molette | Se déplacer |
| `Enter` | Ouvrir le détail du run |
| `r` | Relancer le run |
| `f` | Relancer uniquement les jobs en échec |
| `x` | Annuler le run |
| `o` | Ouvrir le run dans le navigateur |
| `g` | Rafraîchir |

### Détail run

| Touche | Action |
|---|---|
| `l` | Voir les logs |
| `r` | Relancer · `f` relancer les échecs · `x` annuler · `o` ouvrir web |

### Logs

| Touche | Action |
|---|---|
| `↑`/`↓` · `PgUp`/`PgDn` · molette | Défiler le viewport |
| `Esc` | Retour |

---

## Fonctionnalités

- **Tableau de bord hybride** : tes dépôts favoris (avec dernier run, état et
  nombre de runs actifs, en live) suivis des dépôts de l'organisation choisie.
- **Favoris à la volée** : `f` épingle/désépingle le dépôt en surbrillance ; le
  changement est écrit immédiatement dans la config. Un dépôt désépinglé qui
  appartient à l'organisation revient automatiquement dans la liste.
- **Refresh live adaptatif** : rafraîchissement rapide (~3-5 s) tant qu'un run est
  actif, ralenti (~15 s) sinon. Un seul ticker global pour éviter de surcharger
  l'API `gh`.
- **Filtrage instantané** des listes de dépôts (`/`).
- **Pagination + ascenseur vertical + molette** sur toutes les listes (dépôts,
  workflows, runs, branches). La hauteur est paramétrable (`listPageSize`).
- **Formulaire d'inputs dynamique** : les `inputs` du `workflow_dispatch` sont
  parsés depuis le YAML du workflow. Types gérés : `string`, `number`, `boolean`,
  `choice`. Les champs `required` vides bloquent le lancement avec un message.
- **Présélection de branche** : `main` puis `master` sont mises en avant.
- **Bascule auto après dispatch** : `gh workflow run` ne renvoie pas l'ID du run
  créé ; ghrun interroge la liste des runs juste après (avec quelques retries) et
  ouvre directement le détail du run déclenché.
- **Drill-down jobs/étapes** avec icônes de statut (succès / échec / en cours / en
  attente).
- **Logs scrollables** en plein écran (`--log`, ou `--log-failed` si le run a
  échoué).
- **Actions de run** : rerun, rerun-failed, cancel, ouvrir dans le navigateur.
- **Souris** activée partout (molette de défilement sur les listes et les logs).
- **Erreurs non bloquantes** : les échecs `gh` s'affichent en rouge dans le pied de
  page et disparaissent au bout de quelques secondes ; seul un échec d'auth au
  démarrage est fatal.

---

## Configuration

Fichier : `~/.config/ghrun/config.yaml` (respecte `XDG_CONFIG_HOME`).

```yaml
defaultOrg: stephaneHerraiz       # compte/org utilisé pour lister les dépôts
refreshIntervalSeconds: 4         # cadence de base du refresh live
runListLimit: 30                  # nombre de runs récupérés par dépôt
listPageSize: 20                  # nombre de lignes affichées par liste (+ ascenseur)
favorites:                        # dépôts épinglés (statut live sur l'accueil)
  - stephaneHerraiz/ghrun
  - owner/another-repo
```

| Clé | Défaut | Description |
|---|---|---|
| `defaultOrg` | *(choisi au 1ᵉʳ lancement)* | Compte ou organisation dont les dépôts sont listés. |
| `refreshIntervalSeconds` | `4` | Cadence de base du rafraîchissement live. |
| `runListLimit` | `30` | Nombre de runs récupérés par dépôt. |
| `listPageSize` | `20` | Hauteur (en lignes) des listes avant pagination/ascenseur. |
| `favorites` | `[]` | Liste de dépôts `owner/name` épinglés. Modifiable aussi via la touche `f`. |

**Cache** : la liste des dépôts de l'organisation est mise en cache dans
`~/.cache/ghrun/repos.json` (respecte `XDG_CACHE_HOME`) pour un affichage instantané,
puis rafraîchie en arrière-plan.

---

## Comment ghrun parle à GitHub

ghrun n'invente rien : il enveloppe le CLI `gh` (et son API). Vue d'ensemble :

| Fonction | Commande sous-jacente |
|---|---|
| Auth (démarrage) | `gh auth status` |
| Organisations / compte | `gh api user`, `gh api user/orgs` |
| Liste des dépôts | `gh repo list <org> --json nameWithOwner` |
| Workflows d'un dépôt | `gh workflow list` |
| Inputs d'un workflow | contenu du fichier YAML → `on.workflow_dispatch.inputs` |
| Branches | `gh api repos/{o}/{r}/branches` |
| **Dispatch** | `gh workflow run {id} --ref {branche} -f clé=valeur …` |
| Runs (liste live) | `gh run list` |
| Détail d'un run | `gh run view {id} --json status,conclusion,jobs` |
| Logs | `gh run view {id} --log` / `--log-failed` |
| rerun / rerun-failed / cancel | `gh run rerun {id}` / `--failed` / `gh run cancel {id}` |
| Ouvrir web | `gh run view {id} --web` |

Tous les appels sont **asynchrones** (via les commandes Bubbletea) : l'interface ne
bloque jamais.

---

## Hors périmètre

Volontairement non couvert (YAGNI) :

- Édition de workflows ou de secrets.
- Gestion / téléchargement d'artefacts.
- GitHub Enterprise multi-hôte (ghrun cible `github.com` via l'auth `gh` existante).
- Notifications système.
