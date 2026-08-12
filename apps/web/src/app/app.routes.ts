import type { Routes } from "@angular/router";
import { lobbySessionGuard } from "@core/guards/lobby-session.guard";

export const routes: Routes = [
  {
    path: "",
    loadComponent: () =>
      import("./pages/home/home.page").then((module) => module.HomePage),
    title: "Crush Club",
  },
  {
    path: "join",
    loadComponent: () =>
      import("./pages/join/join.page").then((module) => module.JoinPage),
    title: "Rejoindre Crush Club",
  },
  {
    path: "game-configs/new",
    loadComponent: () =>
      import("./pages/game-configs/game-config-form.page").then(
        (module) => module.GameConfigFormPage,
      ),
    title: "Nouveau formulaire Crush Club",
  },
  {
    path: "game-configs/:id/edit",
    loadComponent: () =>
      import("./pages/game-configs/game-config-form.page").then(
        (module) => module.GameConfigFormPage,
      ),
    title: "Modifier le formulaire Crush Club",
  },
  {
    path: "game-configs",
    loadComponent: () =>
      import("./pages/game-configs/game-configs.page").then(
        (module) => module.GameConfigsPage,
      ),
    title: "Mes configurations Crush Club",
  },
  {
    path: "lobby/:code",
    canActivate: [lobbySessionGuard],
    loadComponent: () =>
      import("./pages/lobby/lobby.page").then((module) => module.LobbyPage),
    title: "Lobby Crush Club",
  },
  {
    path: "lobby/:code/photos",
    canActivate: [lobbySessionGuard],
    loadComponent: () =>
      import("./pages/photos/photos.page").then((module) => module.PhotosPage),
    title: "Mes photos Crush Club",
  },
  {
    path: "game/:code/round/:roundNumber/role",
    canActivate: [lobbySessionGuard],
    loadComponent: () =>
      import("./pages/role/role.page").then((module) => module.RolePage),
    title: "Ton rôle Crush Club",
  },
  {
    path: "game/:code/round/:roundNumber/profile",
    canActivate: [lobbySessionGuard],
    loadComponent: () =>
      import("./pages/questionnaire/questionnaire.page").then(
        (module) => module.QuestionnairePage,
      ),
    title: "Profil Crush Club",
  },
  {
    path: "game/:code/round/:roundNumber/review",
    canActivate: [lobbySessionGuard],
    loadComponent: () =>
      import("./pages/review/review.page").then((module) => module.ReviewPage),
    title: "Validation Crush Club",
  },
  {
    path: "game/:code/round/:roundNumber/reveal",
    canActivate: [lobbySessionGuard],
    loadComponent: () =>
      import("./pages/reveal/reveal.page").then((module) => module.RevealPage),
    title: "Révélation Crush Club",
  },
  {
    path: "game/:code/round/:roundNumber/scores",
    canActivate: [lobbySessionGuard],
    loadComponent: () =>
      import("./pages/comparison/comparison.page").then(
        (module) => module.ComparisonPage,
      ),
    title: "Scores finaux Crush Club",
  },
  {
    path: "game/:code/round/:roundNumber/scores/:profileId",
    canActivate: [lobbySessionGuard],
    loadComponent: () =>
      import("./pages/profile-comparison/profile-comparison.page").then(
        (module) => module.ProfileComparisonPage,
      ),
    title: "Comparaison Crush Club",
  },
  {
    path: "game/:code/final",
    canActivate: [lobbySessionGuard],
    loadComponent: () =>
      import("./pages/final-results/final-results.page").then(
        (module) => module.FinalResultsPage,
      ),
    title: "Fin de partie Crush Club",
  },
  {
    path: "game/:code/theme-selection",
    canActivate: [lobbySessionGuard],
    loadComponent: () =>
      import("./pages/theme-selection/theme-selection.page").then(
        (module) => module.ThemeSelectionPage,
      ),
    title: "Thèmes Crush Club",
  },
  {
    path: "game/:code/fast-bio/:roundNumber/assignment",
    canActivate: [lobbySessionGuard],
    loadComponent: () =>
      import("./pages/fast-bio-assignment/fast-bio-assignment.page").then(
        (module) => module.FastBioAssignmentPage,
      ),
    title: "Ta proposition Fast Bio Crush Club",
  },
  {
    path: "game/:code/fast-bio/:roundNumber/review",
    canActivate: [lobbySessionGuard],
    loadComponent: () =>
      import("./pages/fast-bio-review/fast-bio-review.page").then(
        (module) => module.FastBioReviewPage,
      ),
    title: "Revue Fast Bio Crush Club",
  },
  {
    path: "game/:code/fast-bio/final",
    canActivate: [lobbySessionGuard],
    loadComponent: () =>
      import("./pages/fast-bio-final/fast-bio-final.page").then(
        (module) => module.FastBioFinalPage,
      ),
    title: "Classement Fast Bio Crush Club",
  },
  {
    path: "game/:code/zero-to-100/:roundNumber/guess",
    canActivate: [lobbySessionGuard],
    loadComponent: () =>
      import("./pages/zero-to-100-guess/zero-to-100-guess.page").then(
        (module) => module.ZeroToHundredGuessPage,
      ),
    title: "Devine 0 à 100 Crush Club",
  },
  {
    path: "game/:code/zero-to-100/:roundNumber/results",
    canActivate: [lobbySessionGuard],
    loadComponent: () =>
      import("./pages/zero-to-100-results/zero-to-100-results.page").then(
        (module) => module.ZeroToHundredResultsPage,
      ),
    title: "Résultats 0 à 100 Crush Club",
  },
  {
    path: "game/:code/zero-to-100/final",
    canActivate: [lobbySessionGuard],
    loadComponent: () =>
      import("./pages/zero-to-100-final/zero-to-100-final.page").then(
        (module) => module.ZeroToHundredFinalPage,
      ),
    title: "Classement 0 à 100 Crush Club",
  },
  {
    path: "**",
    redirectTo: "",
  },
];
