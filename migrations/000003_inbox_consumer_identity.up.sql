ALTER TABLE inbox_messages
    ADD COLUMN IF NOT EXISTS consumer_name TEXT;

UPDATE inbox_messages
SET consumer_name = 'legacy-consumer'
WHERE consumer_name IS NULL;

ALTER TABLE inbox_messages
    ALTER COLUMN consumer_name SET NOT NULL;

ALTER TABLE inbox_messages
    DROP CONSTRAINT IF EXISTS inbox_messages_pkey;

ALTER TABLE inbox_messages
    ADD CONSTRAINT inbox_messages_pkey PRIMARY KEY (consumer_name, message_id);
