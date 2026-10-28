-- The labels an import last gave a ticket from its source (import_records kind 'ticket'),
-- so a later sync takes off the ones the source has since taken off (contracts/imports.md).
ALTER TABLE import_records ADD COLUMN tag_ids uuid[];
