-- The signed-out OpenCode client resolves its free provider to the current
-- /inference path; the older /zen one now answers every turn with the lane's
-- daily limit. Rewrite only the default the template shipped, so an operator
-- who pointed the connection somewhere else keeps that address.
UPDATE providers
SET base_url = 'https://opencode.ai/inference/openai/v1'
WHERE template_id = 'opencode-free'
  AND base_url = 'https://opencode.ai/zen/v1';
