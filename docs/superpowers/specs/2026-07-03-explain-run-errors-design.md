# ghrun — Explain : explication des erreurs d'un run via RAG local auto-enrichi

Date : 2026-07-03
Statut : validé (brainstorming) — en attente de plan d'implémentation

## Objectif

Depuis l'écran **run detail** d'un run en échec, la touche `e` ouvre un écran
**Explanation** qui explique la cause de l'échec à partir du log du run.

L'explication vient de deux sources, orchestrées automatiquement :

1. **Base de connaissances locale (RAG)** : les erreurs déjà expliquées sont
   stockées avec leur embedding ; une erreur suffisamment similaire (cosine ≥
   seuil) est réexpliquée **instantanément et hors-ligne**.
2. **Claude** : sur miss (aucune entrée assez similaire), ghrun interroge
   Claude — **API Anthropic directe** (SDK Go, modèle `claude-sonnet-5` par
   défaut) si une clé est configurée, **fallback CLI `claude -p`** sinon —
   affiche l'explication, puis **l'embed et la mémorise**. La base s'enrichit
   donc toute seule au fil de l'usage.

Les explications générées sont **en anglais** (cohérent avec le TUI).

## Choix structurants

- **Embeddings : Ollama local** (`http://localhost:11434`), modèle
  `nomic-embed-text` par défaut. Pas d'appel réseau externe pour la recherche.
- **Store vectoriel : [chromem-go](https://github.com/philippgille/chromem-go)**
  (BDD vectorielle embarquée pure-Go, persistée sur disque, requête cosine).
  Une seule collection, persistée sous `~/.config/ghrun/explain-db/`.
- **Explainer : deux implémentations derrière la même interface.**
  1. **API Anthropic directe** (`github.com/anthropics/anthropic-sdk-go`),
     utilisée si une clé est présente (`ANTHROPIC_API_KEY` ou
     `explain.anthropicAPIKey`). Modèle par défaut `claude-sonnet-5`
     (configurable), `max_tokens` 2048. Rapide (~2–5 s), coût ~$0.04–0.06 par
     nouvelle erreur (les hits RAG ne coûtent rien).
  2. **CLI `claude`** en mode non-interactif (`claude -p`), même pattern de
     shell-out que `internal/gh` (interface `Runner` mockable). Coût zéro via
     l'abonnement Claude, plus lent.

  Ordre de tentative sur un miss : API (si clé) → CLI (si binaire présent) ;
  le premier qui répond gagne. Le même prompt système est utilisé par les deux.
- **Déclenchement : auto sur miss.** Hit → local instantané ; miss → `claude`
  automatique. `r` sur l'écran Explanation force une explication `claude`
  fraîche et met à jour la base (upsert).
- **Seuil de similarité : 0.86** par défaut, configurable.
- **Base globale**, partagée entre repos : une erreur `npm ERR!` vue sur un
  repo profite à tous. Chaque entrée garde repo/workflow à titre informatif.

## Flux

```
run detail (conclusion = failure/timed_out) ── e ──▶ écran Explanation

Phase 1 (async) : fetch log (--log-failed, fallback --log)
                  → normalisation → signature sha256
                  → fast-path : signature exacte connue ? → HIT exact
                  → sinon embed (Ollama) + query chromem
                     · similarité ≥ seuil → HIT (badge « 🧠 local · NN% »)
                     · sinon → MISS → Phase 2
Phase 2 (async) : Claude — API Anthropic (si clé) sinon claude -p —
                  log brut tronqué par la fin
                  → affichage (badge « ✨ claude-sonnet-5 » ou « ✨ claude (cli) »)
                  → embed + upsert dans chromem (best-effort)
```

- **HIT** : l'explication stockée est affichée, le compteur d'usage et
  `lastUsedAt` de l'entrée sont mis à jour.
- **`r` (regenerate)** : relance la Phase 2 quel que soit l'état courant ;
  l'upsert (ID = signature du log courant) **corrige** l'entrée si la
  réutilisation locale était à côté.

## Écran Explanation

| Élément | Contenu |
|---|---|
| En-tête | `run #<id> — <repo>` + badge source (`🧠 local · 92%`, `✨ claude-sonnet-5` ou `✨ claude (cli)`) |
| Corps | viewport scrollable (réutilise le pattern de `logs.go`), texte brut |
| États | `Fetching failed log…` / `Searching local knowledge…` / `Asking claude… (may take a minute)` |
| Erreur | message dans le style `errStyle` existant |

Raccourcis (contextuels, minuscules — convention du projet) :

| Touche | Action |
|---|---|
| `r` | regenerate : force une explication `claude` fraîche + upsert |
| `l` | pousse l'écran logs bruts (`--log-failed`) |
| molette / flèches | scroll du viewport |
| retour | convention globale existante (pop de la pile) |

Sur **run detail**, `e` n'est actif (et affiché dans le footer) que si
`Conclusion ∈ {failure, timed_out}`.

## Structure du code

Nouveau package `internal/explain/`, découplé et testable par interfaces
(même philosophie que `GHClient`) :

```
internal/explain/
├── service.go      # Service : orchestre normalize → retrieve → hit/miss → claude → store
├── normalize.go    # extraction région d'erreur + normalisation + signature
├── ollama.go       # Embedder via HTTP Ollama /api/embeddings
├── anthropic.go    # Explainer via l'API Anthropic (SDK Go, claude-sonnet-5)
├── claude.go       # Explainer via shell-out `claude -p` (Runner mockable)
├── chain.go        # Explainer composite : API (si clé) puis CLI en fallback
├── store.go        # Store au-dessus de chromem-go (persistant)
└── *_test.go
internal/ui/
├── explain.go      # écran Explanation (viewport + états + touches)
└── rundetail.go    # + touche `e`, hint footer
```

Interfaces :

```go
type Embedder interface {
    Embed(ctx context.Context, text string) ([]float32, error)
}

type Explainer interface {
    Explain(ctx context.Context, req ExplainRequest) (string, error)
}

type Store interface {
    GetBySignature(sig string) (*Entry, bool)
    Query(embedding []float32, topK int) ([]Match, error) // Match = Entry + Similarity ; le Service n'utilise que le meilleur match (topK = 1)
    Upsert(e Entry) error
    Touch(sig string) error // incrémente useCount + lastUsedAt
}
```

`Service` expose deux étapes asynchrones consommées par l'UI en `tea.Cmd` :

- `ResolveLocal(...)` → `explainLocalMsg{result | miss, err}`
- `AskClaude(...)`    → `explainClaudeMsg{result, err}` (inclut l'upsert)

Le câblage (construction du Service avec config) se fait dans `bootstrap.go`,
comme le reste.

## Normalisation & données stockées

Depuis le log échoué, `normalize.go` :

1. **Extrait la région d'erreur** : lignes matchant (insensible à la casse)
   `##[error]`, `error`, `err!`, `fail` / `failed` / `failure` / `failing`,
   `fatal`, `panic`, `✗`, `exit code`, `Process completed with exit code`,
   plus N lignes de contexte avant chacune (N = 5). Heuristique volontairement
   large : mieux vaut sur-extraire que rater la cause racine.
2. **Normalise les tokens volatils** → placeholders : timestamps, dates,
   durées, chemins absolus (`/home/…`, `/tmp/…`, `/__w/…`), UUID, SHA (7–40
   hex), adresses IP, ports, numéros de ligne dans les traces, dossiers
   temporaires, IDs numériques longs.
3. Produit :
   - `normalizedText` — ce qui est **embeddé** et comparé ;
   - `signature` — sha256 de `normalizedText` (fast-path exact, ID chromem).

Document chromem (une entrée = une erreur expliquée) :

| Champ chromem | Contenu |
|---|---|
| `ID` | signature sha256 |
| `Content` | `normalizedText` (texte embeddé) |
| `Embedding` | vecteur Ollama |
| `Metadata` | `explanation`, `repo`, `workflow`, `failedSteps`, `model`, `createdAt`, `lastUsedAt`, `useCount`, `language` |

Ce qui est envoyé à Claude (API comme CLI) : le log échoué **brut** (non normalisé, plus
lisible pour le LLM), tronqué **par la fin** à `maxLogBytes` (les erreurs sont
en fin de log), précédé d'un prompt système fixe : *« Explain why this GitHub
Actions run failed. Be concise: root cause first, then the failing
step/command, then a suggested fix. Answer in English. »* (la langue vient de
la config).

## Config — `~/.config/ghrun/config.yaml`

Nouvelle section `explain`, tous les champs optionnels avec défauts :

```yaml
explain:
  enabled: true
  ollamaURL: http://localhost:11434
  embeddingModel: nomic-embed-text
  similarityThreshold: 0.86
  anthropicAPIKey: ""       # optionnel — sinon lue depuis ANTHROPIC_API_KEY (env)
  model: claude-sonnet-5    # modèle utilisé par l'Explainer API
  claudeCmd: claude         # fallback CLI si aucune clé API
  storePath: ~/.config/ghrun/explain-db
  maxLogBytes: 65536        # 64 KiB, tronqué par la fin
  language: English
```

## Dégradation gracieuse

| Situation | Comportement |
|---|---|
| Ollama injoignable | Skip du RAG (pas d'embedding possible) → appel `claude` direct ; warn « knowledge base disabled (Ollama unreachable) » ; **pas de stockage** (pas de vecteur) |
| Appel API en erreur (réseau, 4xx/5xx) ou pas de clé | Fallback automatique sur le CLI `claude` s'il est présent |
| API **et** CLI indisponibles | Si un match local sous le seuil existe → affiché en « best guess (NN%) » ; sinon message clair + suggestion d'ouvrir les logs (`l`) |
| Ollama **et** tous les explainers absents | Message clair + logs bruts accessibles via `l` |
| Log échoué vide (`--log-failed`) | Fallback `--log` complet ; si toujours vide → message |
| Store corrompu / illisible | Recréé vide (warn), la feature reste fonctionnelle |
| `explain.enabled: false` | `e` inactif, pas de hint dans le footer |

Toutes les erreurs sont non-fatales : jamais de crash du TUI à cause de la
feature (cohérent avec la gestion `errMsg` existante).

## Tests

- **normalize** : table-driven — extraction de la région d'erreur (y compris
  variantes de casse : `FAIL`, `failed`, `Failure`…), chaque classe de token
  volatil, stabilité de la signature (deux logs identiques aux tokens volatils
  près → même signature).
- **Service** : Embedder/Explainer/Store factices → hit exact, hit
  par similarité, miss → claude → upsert, regenerate → upsert, Ollama down →
  claude direct sans stockage, claude down → best guess.
- **Explainer API** : serveur `httptest` factice (réponse, erreur 4xx/5xx,
  timeout). **Chain** : clé présente → API ; API en échec → fallback CLI ;
  pas de clé → CLI direct.
- **Store** : aller-retour chromem sur répertoire temporaire (upsert, query,
  touch, réouverture).
- **UI explain** : pattern des tests d'écran existants (`logs_test.go`) —
  états de chargement, affichage hit/miss, touche `r`, touche `l`.

Aucun test ne requiert Ollama ni `claude` réels.

## Hors scope (YAGNI)

- Rendu markdown riche (`glamour`) — texte brut d'abord.
- Explication depuis le dashboard ou la liste des runs (run detail seulement).
- Synchronisation/partage de la base entre machines.
- Notation/feedback des explications (👍/👎).
- Fine-tuning ou LLM local génératif — le « modèle local » est le couple
  embeddings + base, pas un LLM.
- Nettoyage/éviction automatique de la base (taille négligeable à l'échelle
  d'un usage perso).
