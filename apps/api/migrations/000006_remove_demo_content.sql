-- +goose Up
UPDATE questions
SET description = 'Comment cette personne montre-t-elle son affection ?'
WHERE id = 'love-language';

UPDATE lobbies
SET game_config_snapshot = jsonb_set(
      game_config_snapshot,
      '{questions}',
      (
        SELECT jsonb_agg(
          CASE
            WHEN question ->> 'id' = 'love-language'
              THEN jsonb_set(
                question,
                '{description}',
                to_jsonb('Comment cette personne montre-t-elle son affection ?'::text)
              )
            ELSE question
          END
          ORDER BY ordinal
        )
        FROM jsonb_array_elements(game_config_snapshot -> 'questions')
          WITH ORDINALITY AS item(question, ordinal)
      ),
      true
    ),
    revision = revision + 1,
    updated_at = now()
WHERE game_config_snapshot IS NOT NULL
  AND jsonb_typeof(game_config_snapshot -> 'questions') = 'array'
  AND EXISTS (
    SELECT 1
    FROM jsonb_array_elements(game_config_snapshot -> 'questions') AS question
    WHERE question ->> 'id' = 'love-language'
  );

-- +goose Down
SELECT 1;
