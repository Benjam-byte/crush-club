-- name: ListActiveQuestions :many
SELECT * FROM questions WHERE is_active ORDER BY created_at, id;

-- name: ListVisibleGameConfigs :many
SELECT *
FROM game_configs
WHERE is_system OR owner_identity_id = $1 OR is_public
ORDER BY is_system DESC, (owner_identity_id = $1) DESC, updated_at DESC, name ASC;

-- name: ListGameConfigQuestions :many
SELECT questions.*
FROM game_config_questions
JOIN questions ON questions.id = game_config_questions.question_id
WHERE game_config_questions.game_config_id = $1
ORDER BY game_config_questions.position;
