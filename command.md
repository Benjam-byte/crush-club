# Commandes de test

Toutes les commandes se lancent depuis la racine du projet (`crushclub/`), sauf quand un `cd` est indiqué.
Les exemples sont écrits pour PowerShell (Windows).

## Lancer l'application

| Commande | Ce que ça fait |
|---|---|
| `corepack pnpm dev:full` | Démarre Docker (PostgreSQL, migrations, API, nginx), attend l'API, puis lance Angular. `Ctrl+C` arrête tout et garde les données. |
| `docker compose up -d --build` | Démarre seulement la stack Docker en arrière-plan. Elle reste lancée après avoir coupé Angular. |
| `corepack pnpm dev` | Lance seulement Angular. La stack Docker doit déjà tourner. |
| `docker compose down` | Arrête la stack Docker et garde les données. |

Ouvre le jeu sur **http://localhost:4200**. Le port 8080 sert à l'API et aux bots. Dans le navigateur, utilise le 4200.

## Bots (mode Situation uniquement)

```powershell
corepack pnpm bots:situation <CODE> [--bots 3] [--api http://localhost:8080]
```

| Option | Valeur |
|---|---|
| `<CODE>` | Code du lobby affiché à l'écran. Majuscules ou minuscules, peu importe. |
| `--bots` | Nombre de bots, de 1 à 9 (3 par défaut). |
| `--api` | Adresse de l'API (`http://localhost:8080` par défaut, c'est la stack Docker). |

Déroulé :

1. Lance l'application, puis crée un lobby en mode **Situation** dans le navigateur. Tu es l'hôte.
2. Dans un second terminal, ajoute les bots :
   ```powershell
   corepack pnpm bots:situation ABCD --bots 4
   ```
3. Lance la partie depuis le navigateur.
4. Les bots jouent seuls, avec 0,8 à 2,5 s de « réflexion » par action :
   - ils proposent un thème, puis classent les thèmes ;
   - ils font leur proposition à chaque manche ;
   - ils votent aux duels et classent les finalistes.
5. Les actions d'hôte restent à toi : lancer la partie et les manches suivantes.
6. Fais `Ctrl+C` dans le terminal des bots pour les déconnecter.

À savoir :

- Il n'existe pas de bots pour les modes Classique, Fast Bio, 0 à 100 et Wavelength.
- Le mode Situation démarre dès 2 joueurs : un seul bot suffit pour tester.
- Les bots laissent le temps de voir les écrans d'attente et leurs mini-jeux (« Proposition envoyée ! », « Classement envoyé ! »).

## Plusieurs vrais joueurs sur un seul PC

La session d'un joueur est enregistrée dans le navigateur. Il faut donc **un navigateur ou un profil par joueur** :

- une fenêtre normale ;
- une fenêtre de navigation privée. Toutes les fenêtres privées partagent une seule session ;
- un autre navigateur (Edge, Firefox…).

Pour simuler un téléphone, ouvre les outils de développement (`F12`) puis `Ctrl+Shift+M`.

## Tester sur un vrai téléphone (même Wi-Fi)

```powershell
docker compose up -d --build
cd apps/web
npx ng serve --proxy-config proxy.conf.json --host 0.0.0.0
```

1. Trouve l'adresse IPv4 du PC avec `ipconfig`, par exemple `192.168.1.20`.
2. Sur le téléphone, ouvre `http://192.168.1.20:4200`.
3. Si Windows demande d'autoriser Node.js dans le pare-feu, accepte pour les réseaux privés.

Cette adresse est en `http`, pas en `https`. L'installation en PWA et le service worker ne sont donc pas actifs. Pour tester l'app installée sur iPhone, utilise l'URL déployée sur Coolify.

## Tests automatiques

### Front (Vitest)

| Commande | Ce que ça fait |
|---|---|
| `corepack pnpm test` | Lance tous les tests du front. |
| `cd apps/web; npx vitest run waiting-game` | Lance seulement les tests dont le chemin contient `waiting-game`. Remplace par le nom que tu veux. |
| `cd apps/web; npx vitest` | Mode watch : relance les tests à chaque sauvegarde. |

### Qualité du code

```powershell
corepack pnpm lint
corepack pnpm typecheck
corepack pnpm build
```

Le build affiche un avertissement de budget sur le bundle initial (1,13 MB pour 1 MB autorisé). Il est connu et ne bloque pas le build.

### API Go

Tests unitaires :

```powershell
cd apps/api
go test ./...
```

Tests d'intégration. Ils jouent une vraie partie multijoueur contre la stack Docker, qui doit donc être lancée :

```powershell
cd apps/api
$env:API_INTEGRATION_URL = "http://localhost:8080"
go test ./cmd/api -run '^TestMultiplayer' -count=1 -v
```

En bash (Git Bash, WSL) : `API_INTEGRATION_URL=http://localhost:8080 go test ./cmd/api -run '^TestMultiplayer' -count=1 -v`

## Docker au quotidien

| Commande | Ce que ça fait |
|---|---|
| `docker compose ps` | Affiche l'état de chaque service. |
| `docker compose logs -f api` | Affiche les logs de l'API en direct (niveau réglé par `LOG_LEVEL` dans `.env`). |
| `docker compose up -d --build api` | Reconstruit et redémarre seulement l'API, après une modification du Go. |
| `docker compose run --rm migrate` | Rejoue les migrations de la base. |
| `docker compose exec postgres psql -U crushclub -d crushclub` | Ouvre une console SQL. Les identifiants viennent de `.env`. |
| `docker compose down -v` | ⚠️ **Efface la base et les photos** en supprimant les volumes. À utiliser pour repartir de zéro. |

## Dans le navigateur

- **Page de diagnostic** (WebSocket, compression, latences) : tape 4 fois rapidement sur le logo de l'accueil, ou ouvre http://localhost:4200/diagnostics.
- **Remettre à zéro les records des mini-jeux** : dans la console des outils de développement, tape
  ```js
  localStorage.removeItem('crush-club-waiting-game-best-v1')
  ```
- **Changer de mini-jeu** : recharge la page pour tirer un nouveau jeu au hasard. D'un écran d'attente à l'autre, le jeu change toujours, mais après un rechargement le même jeu peut revenir.
