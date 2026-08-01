import type { Routes } from "@angular/router";

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
    loadComponent: () =>
      import("./pages/lobby/lobby.page").then((module) => module.LobbyPage),
    title: "Lobby Crush Club",
  },
  {
    path: "lobby/:code/photos",
    loadComponent: () =>
      import("./pages/photos/photos.page").then((module) => module.PhotosPage),
    title: "Mes photos Crush Club",
  },
  {
    path: "game/demo/role",
    loadComponent: () =>
      import("./pages/role/role.page").then((module) => module.RolePage),
    title: "Ton rôle Crush Club",
  },
  {
    path: "game/demo/round/1",
    loadComponent: () =>
      import("./pages/questionnaire/questionnaire.page").then(
        (module) => module.QuestionnairePage,
      ),
    title: "Profil de Camille Crush Club",
  },
  {
    path: "game/demo/round/1/review",
    loadComponent: () =>
      import("./pages/review/review.page").then((module) => module.ReviewPage),
    title: "Validation Crush Club",
  },
  {
    path: "game/demo/reveal/1",
    loadComponent: () =>
      import("./pages/reveal/reveal.page").then((module) => module.RevealPage),
    title: "Révélation Crush Club",
  },
  {
    path: "game/demo/reveal/1/scores",
    loadComponent: () =>
      import("./pages/comparison/comparison.page").then(
        (module) => module.ComparisonPage,
      ),
    title: "Scores finaux Crush Club",
  },
  {
    path: "game/demo/reveal/1/comparison/:profileId",
    loadComponent: () =>
      import("./pages/profile-comparison/profile-comparison.page").then(
        (module) => module.ProfileComparisonPage,
      ),
    title: "Comparaison Crush Club",
  },
  {
    path: "game/demo/reveal/1/comparison",
    redirectTo: "game/demo/reveal/1/scores",
    pathMatch: "full",
  },
  {
    path: "game/demo/results/1",
    loadComponent: () =>
      import("./pages/round-results/round-results.page").then(
        (module) => module.RoundResultsPage,
      ),
    title: "Résultats de la manche Crush Club",
  },
  {
    path: "game/demo/final",
    loadComponent: () =>
      import("./pages/final-results/final-results.page").then(
        (module) => module.FinalResultsPage,
      ),
    title: "Fin de partie Crush Club",
  },
  {
    path: "**",
    redirectTo: "",
  },
];
