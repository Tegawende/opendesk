-- A view's icon in the sidebar (contracts/api.md, `ViewIcon`): {kind, value, color}, where kind is
-- emoji, icon (a name of the app's list) or text (one or two characters), and color a
-- palette colour or null. NULL shows the default icon. Checked by the API.
ALTER TABLE views ADD COLUMN icon jsonb CHECK (icon IS NULL OR jsonb_typeof(icon) = 'object');
