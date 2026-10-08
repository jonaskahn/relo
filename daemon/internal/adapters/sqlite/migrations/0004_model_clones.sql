-- A model an operator clones keeps its own identifier while Relo sends the
-- upstream id to the provider, which is how an operator reaches a model the
-- provider has not listed yet. cloned_from names the model a clone was copied
-- from, and stays empty on every row a listing or an operator added directly.
ALTER TABLE models ADD COLUMN upstream_model_id TEXT NOT NULL DEFAULT '';
ALTER TABLE models ADD COLUMN cloned_from TEXT NOT NULL DEFAULT '';
