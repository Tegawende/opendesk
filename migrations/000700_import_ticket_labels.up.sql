-- The labels an import last gave a ticket from its source (import_records kind 'ticket'),
-- so a later sync takes off the ones the source has since taken off (contracts/imports.md).
ALTER TABLE import_records ADD COLUMN tag_ids uuid[];

-- Ownership belongs to the stable ticket/label relation, including after a rename or merge.
-- Keep existing relations: their original ownership cannot be inferred from display names.
ALTER TABLE ticket_tags ADD COLUMN local_owned boolean NOT NULL DEFAULT true;
