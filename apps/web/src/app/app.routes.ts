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
    path: "game/:code/fast-bio/themes",
    canActivate: [lobbySessionGuard],
    loadComponent: () =>
      import("./pages/fast-bio-themes/fast-bio-themes.page").then(
        (module) => module.FastBioThemesPage,
      ),
    title: "Thèmes Fast Bio Crush Club",
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
    path: "**",
    redirectTo: "",
  },
];
