# Validation du layout mobile

Contrôles effectués le 9 novembre 2026, sur Chrome local en 375 × 667 et 390 × 844.

## Affichage

160 vues et états ont été mesurés dans les composants Angular réels avec des données de contrôle :

- accueil, connexion, création, bibliothèque et éditeur ;
- lobby classique et Situation, hôte et invité, entracte, admission en attente ;
- photos à 0, 2 et 4 emplacements remplis ;
- classique : rôles, étapes du questionnaire, validation, attente, révélation, scores et détail ;
- proposition et classement des thèmes, avant et après envoi ;
- Fast Bio, 0 à 100, Situation, Wavelength et classement secret, y compris les attentes et les finales hôte/invité.

Résultats : aucun défilement de `ion-content`, aucun dépassement du corps ni débordement horizontal
dans ces vues, sauf le défilement volontaire de l’éditeur. Les listes ont leur propre zone de défilement.
Les boutons du footer mesurent 48 px et restent dans le viewport. Aucune exception JavaScript relevée.

Cas limites supplémentaires vérifiés : vingt choix dans une question ; cinq nominés superposés sur les
deux échelles de résultats de 0 à 100 ; listes de dix joueurs ; thèmes longs. Les noms, rangs et valeurs
des échelles compactes restent disponibles dans leur description accessible.

### Petits écrans et texte agrandi (10 novembre 2026)

L’émulation Chrome n’affiche pas les barres de Safari : un iPhone SE dans Safari n’offre que 375 × 553.
Les écrans d’entrée (accueil, rejoindre, créer, bibliothèque, éditeur) tiennent sans défilement en
375 × 667, 390 × 664, 360 × 640, 375 × 553 et 320 × 568.

`.screen-body` défile désormais en secours au lieu de couper son contenu : avec le texte à 125 %
(réglage d’accessibilité) en 375 × 553, la création de lobby dépasse de 88 px et reste lisible en
défilant, sans débordement latéral. Les listes `.screen-scroll` gardent 6rem minimum dans ce cas.

### 0 à 10 (10 novembre 2026)

Le mode « 0 à 100 » est devenu « 0 à 10 » : notes entières, podium du groupe, carrousel des réponses.
Vraie partie locale à cinq joueurs (cinq nominés, un nom de 24 caractères), jouée par l’API sur trois
manches jusqu’au classement final, avec captures Chrome headless des écrans réels :

- saisie en 360 × 640 ;
- résultats hôte et invité en 390 × 844, 360 × 640, 375 × 553 et 320 × 568 : aucun défilement, aucun
  débordement latéral, footer dans le viewport ; en dessous de sa hauteur utile, le carrousel descend à
  8rem et ses cartes défilent ;
- texte à 125 % en 375 × 553 : le corps défile de 42 px, sans débordement latéral ;
- égalités (rangs partagés), ordre parfait (carte dorée), grand écart et classement final vérifiés.

## Interactions et parties

- Mini-jeux : canvas au ratio 4:3 ; modale sans breakpoints, restant ouverte après un swipe ; fermeture explicite opérationnelle.
- Règles : sheet avec breakpoints 0 / 0,75 / 1 et contenu projeté visible.
- Thèmes : réordonnancement par la poignée et défilement automatique CDK vérifiés à la souris.
- Formulaires : soumission par un bouton Ionic placé hors du formulaire vérifiée.
- Éditeur : une seule question dépliée après ajout ou changement de question.
- Situation : vraie partie locale à dix joueurs, avec `bots:situation --bots 9`, jouée sur trois manches jusqu’au classement final en 375 × 667, sans erreur API ni dépassement de page.
- Revanche classique : test HTTP/WebSocket à deux joueurs, fin de partie complète, autorisation hôte, refus avant la fin ou si un joueur est déconnecté, double requête concurrente, conservation des joueurs/photos et synchronisation de l’invité.
- Brouillon de revanche : le même composant questionnaire conservé par Ionic revient à l’étape 1 avec photo, accroche et réponses vides ; la clé de brouillon change avec l’identifiant de partie.

## Vérifications automatisées

```powershell
corepack pnpm --filter @crush-club/web typecheck
corepack pnpm --filter @crush-club/web test
corepack pnpm --filter @crush-club/web lint
corepack pnpm --filter @crush-club/web build
# Après le build, Chrome local (CHROME_PATH si introuvable) : écrans d’entrée en 5 formats + texte 125 %
corepack pnpm layout:check
# Depuis apps/api :
go test ./...
$env:API_INTEGRATION_URL = 'http://localhost:8080'
go test ./cmd/api -run TestMultiplayerClassicReplayKeepsGroupAndResetsGame -count=1 -v
```

Toutes passent : 164 tests web. Le build signale un avertissement non bloquant sur le bundle initial
(1,17 Mo pour un budget d’avertissement de 1 Mo). `layout:check` ne couvre que les écrans accessibles
sans partie : les écrans de jeu demandent l’API et plusieurs joueurs.

## Limites du contrôle

L’émulation Chrome ne remplace pas un iPhone physique. Le clavier Safari, la PWA, les safe areas
et les gestes tactiles natifs restent à vérifier sur appareil. Les données de contrôle couvrent la mise
en page ; elles ne remplacent pas un parcours multijoueur réel dans chacun des six modes.
