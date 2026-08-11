# Crush Club

PWA mobile-first de faux profils entre amis, réservée à des groupes d'adultes.

Le frontend suit les conventions locales du dossier `skills/` :

- Angular 20+ et composants standalone ;
- Ionic Angular pour la structure mobile ;
- signals et `computed()` pour l'état ;
- Tailwind CSS en priorité, SCSS par composant pour les besoins Ionic et les effets complexes ;
- architecture `core/` et `pages/`.

Le backend Go, PostgreSQL, les migrations et Docker Compose proviennent du pack produit fourni dans `crush-club-claude-code-pack/`.

## Démarrage

```bash
corepack enable
pnpm install
pnpm dev
```

L'application est disponible sur `http://localhost:4200`.

Pour démarrer automatiquement Docker, PostgreSQL, les migrations, l’API et Angular dans le bon ordre :

```bash
corepack pnpm dev:full
```

La commande attend que l’API soit prête avant de lancer Angular. `Ctrl+C` arrête la stack sans supprimer
les volumes PostgreSQL.

## Validation

```bash
pnpm lint
pnpm typecheck
pnpm test
pnpm build

cd apps/api
go test ./...
```

Le parcours d'intégration réel s'exécute contre la stack Docker exposée sur le port 8080 :

```bash
cd apps/api
API_INTEGRATION_URL=http://localhost:8080 go test ./cmd/api -run '^TestMultiplayer' -count=1 -v
```

Il couvre notamment quatre joueurs et quatre manches dans l'ordre d'arrivée, les trois entractes, les quatre
photos par joueur, la confidentialité, les snapshots WebSocket, la reconnexion, les scores de manche et les
totaux persistants, ainsi que les connexions concurrentes jusqu'à la capacité maximale.

## Docker

```bash
cp .env.example .env
docker compose up --build
```

Le service web est exposé sur `http://localhost:8080`. Nginx relaie `/api` et `/ws` vers l'API Go.

## Multijoueur temps réel

PostgreSQL est la source de vérité des participants, manches, soumissions, votes et classements. Les
commandes passent par REST avec le jeton joueur dans `Authorization: Bearer ...`; `/ws/lobbies/:code`
demande ce même jeton dans son premier message, puis diffuse des snapshots personnalisés et révisionnés.
Le navigateur ne garde que sa session de reconnexion et ses brouillons non envoyés.

Une partie accepte 2 à 10 joueurs et démarre quand tous les joueurs connectés ont envoyé exactement quatre
photos. Chaque participant devient la cible d'une manche dans l'ordre d'arrivée. La cible fournit les
réponses officielles et choisit anonymement une accroche; les autres envoient une prédiction et leur LOVER.
Les photos sont ajoutées et persistées une par une, puis validées ensemble. Le client web convertit les
formats HEIC/HEIF et normalise les images en JPEG avant leur envoi afin de prendre en charge les photothèques
iPhone et les clichés haute résolution.
Après chaque résultat, l'hôte ramène tout le groupe au lobby d'entracte, puis lance manuellement la manche
suivante. La dernière manche ouvre le classement final. Une revanche crée toujours un nouveau lobby, de
nouvelles sessions et exige quatre nouvelles photos par joueur.
Les photos privées et les lobbies expirent après 24 heures. Une déconnexion conserve la place 90 secondes,
puis autorise l'exclusion et, pour l'hôte, le transfert de ses droits au plus ancien joueur connecté.

## Déploiement Coolify

Le fichier `compose.coolify.yaml` déploie le frontend Angular/Nginx, l'API Go, les migrations et
PostgreSQL dans une seule ressource Docker Compose. Dans Coolify, sélectionnez ce fichier, renseignez
les variables de `.env.example`, puis associez uniquement le service `web` au domaine HTTPS public.
L'image API Coolify applique les migrations avant chaque démarrage. Les services `api` et `postgres`
restent internes à la stack. Les volumes `postgres-data` et `photo-data` persistent entre les déploiements.
Sans nom de domaine personnel, `SERVICE_FQDN_WEB_80` demande à Coolify de générer une adresse gratuite
`sslip.io`. L'URL du service `web` doit utiliser `https://` afin que les cookies sécurisés fonctionnent.

## Configurations de jeu

Les questions sont stockées dans PostgreSQL et le preset `Classique` est créé par la migration
`000002_game_configs.sql`. Depuis `/game-configs`, l’hôte dispose d’un éditeur de formulaire : il peut
ajouter, supprimer et réordonner des questions personnalisées de type plage numérique, liste ou oui/non.
La migration `000003_custom_questions.sql` ajoute leur propriété à la session hôte anonyme. Le preset
système reste disponible et peut être dupliqué : ses questions deviennent alors des copies personnelles
entièrement modifiables. Les questionnaires personnels et partagés ne contiennent que la photo principale,
la phrase d’accroche et leurs questions personnalisées ; les cinq champs de bio restent propres au preset
`Classique`. La configuration est ensuite choisie lors de la création du lobby.

Un questionnaire personnel est privé par défaut. Son propriétaire peut activer « Partager avec tout le
monde » : il reste propriétaire du modèle, tandis que les autres hôtes peuvent le sélectionner pour un
lobby ou le dupliquer sans pouvoir modifier ni supprimer l’original. Cette visibilité est ajoutée par la
migration `000004_shared_game_configs.sql`.

Chaque lobby conserve un snapshot JSON de la configuration sélectionnée. Une configuration personnelle
peut donc être modifiée ou supprimée sans changer les parties qui l’utilisent déjà. La bibliothèque est
liée à un cookie hôte HTTP-only anonyme et reste propre au navigateur utilisé.

En développement, lancez l’API et PostgreSQL avant `pnpm dev`; le serveur Angular relaie automatiquement
`/api` vers `http://localhost:8080` grâce à `apps/web/proxy.conf.json`.
