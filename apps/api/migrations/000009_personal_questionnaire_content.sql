-- +goose Up
-- Materialize every system question referenced by a personal configuration as
-- an owned copy. Existing lobby snapshots intentionally remain untouched.
UPDATE game_configs AS config
SET version = config.version + 1,
    updated_at = now()
WHERE NOT config.is_system
  AND EXISTS (
    SELECT 1
    FROM game_config_questions AS item
    JOIN questions AS question ON question.id = item.question_id
    WHERE item.game_config_id = config.id AND question.is_system
  );

-- +goose StatementBegin
DO $$
DECLARE
  linked_question record;
  copied_question_id text;
BEGIN
  FOR linked_question IN
    SELECT item.game_config_id, item.question_id, config.owner_identity_id
    FROM game_config_questions AS item
    JOIN game_configs AS config ON config.id = item.game_config_id
    JOIN questions AS question ON question.id = item.question_id
    WHERE NOT config.is_system AND question.is_system
    ORDER BY item.game_config_id, item.position
  LOOP
    copied_question_id := gen_random_uuid()::text;

    INSERT INTO questions (
      id, type, label, description, maximum_score, lover_eligible, options,
      minimum, maximum, minimum_label, maximum_label, is_active,
      owner_identity_id, is_system
    )
    SELECT
      copied_question_id, type, label, description, maximum_score, lover_eligible, options,
      minimum, maximum, minimum_label, maximum_label, is_active,
      linked_question.owner_identity_id, false
    FROM questions
    WHERE id = linked_question.question_id;

    UPDATE game_config_questions
    SET question_id = copied_question_id
    WHERE game_config_id = linked_question.game_config_id
      AND question_id = linked_question.question_id;
  END LOOP;
END $$;
-- +goose StatementEnd

-- +goose Down
-- Irreversible: copied questions may have been edited independently afterward.
SELECT 1;
