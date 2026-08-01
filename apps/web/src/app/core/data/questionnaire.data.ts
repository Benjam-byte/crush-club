import type { BioCategory, QuestionDefinition } from '@core/models/game.models';

export const bioCategoryList: readonly BioCategory[] = [
  {
    id: 'quality',
    label: 'Qualité',
    optionList: [
      { id: 'attentive', label: 'Attentionnée' },
      { id: 'funny', label: 'Drôle' },
      { id: 'ambitious', label: 'Ambitieuse' },
      { id: 'spontaneous', label: 'Spontanée' },
    ],
  },
  {
    id: 'flaw',
    label: 'Défaut',
    optionList: [
      { id: 'stubborn', label: 'Têtue' },
      { id: 'impatient', label: 'Impatiente' },
      { id: 'late', label: 'Toujours en retard' },
      { id: 'perfectionist', label: 'Perfectionniste' },
    ],
  },
  {
    id: 'passion',
    label: 'Passion',
    optionList: [
      { id: 'travel', label: 'Voyage' },
      { id: 'music', label: 'Musique' },
      { id: 'cooking', label: 'Cuisine' },
      { id: 'photography', label: 'Photographie' },
    ],
  },
  {
    id: 'lifestyle',
    label: 'Style de vie',
    optionList: [
      { id: 'sporty', label: 'Sportive' },
      { id: 'homebody', label: 'Casanière' },
      { id: 'adventurous', label: 'Aventurière' },
      { id: 'zen', label: 'Zen' },
    ],
  },
  {
    id: 'intention',
    label: 'Je recherche',
    optionList: [
      { id: 'serious', label: 'Relation sérieuse' },
      { id: 'complicity', label: 'Complicité' },
      { id: 'light', label: 'Aventure légère' },
      { id: 'see', label: 'Voir où ça mène' },
    ],
  },
];

export const questionList: readonly QuestionDefinition[] = [
  {
    id: 'romance',
    type: 'integer_range',
    label: 'Niveau de romantisme',
    description: 'De 0 « pas du tout » à 10 « cœur en marshmallow »',
    maximumScore: 10,
    loverEligible: true,
    minimum: 0,
    maximum: 10,
    minimumLabel: 'Discret',
    maximumLabel: 'Très romantique',
  },
  {
    id: 'love-language',
    type: 'single_choice',
    label: "Langage de l'amour",
    description: 'Comment Camille montre-t-elle son affection ?',
    maximumScore: 10,
    loverEligible: true,
    options: [
      { id: 'words', label: 'Mots valorisants' },
      { id: 'time', label: 'Moments de qualité' },
      { id: 'acts', label: 'Petites attentions' },
      { id: 'touch', label: 'Contact physique' },
    ],
  },
  {
    id: 'first-date',
    type: 'single_choice',
    label: 'Son rendez-vous idéal',
    description: 'Le programme qui lui ressemble le plus',
    maximumScore: 10,
    loverEligible: true,
    options: [
      { id: 'restaurant', label: 'Un dîner intimiste' },
      { id: 'picnic', label: 'Un pique-nique improvisé' },
      { id: 'concert', label: 'Un concert' },
      { id: 'escape', label: 'Un escape game' },
    ],
  },
  {
    id: 'weekend',
    type: 'binary_choice',
    label: 'Pour un week-end à deux',
    description: 'Elle préfère…',
    maximumScore: 8,
    loverEligible: true,
    options: [
      { id: 'planned', label: 'Tout organiser' },
      { id: 'improvise', label: 'Tout improviser' },
    ],
  },
  {
    id: 'intimacy',
    type: 'integer_range',
    label: 'Importance de parler de ses envies',
    description: 'Une question intime glissée naturellement dans le jeu',
    maximumScore: 10,
    loverEligible: true,
    minimum: 0,
    maximum: 10,
    minimumLabel: 'Peu important',
    maximumLabel: 'Essentiel',
  },
];
