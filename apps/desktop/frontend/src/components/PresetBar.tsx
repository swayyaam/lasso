import { useState } from "react";
import { Button, Chip, EmptyState, Eyebrow, Input, cx } from "@lasso/ui";
import { api, core, presets as presetModels } from "../bindings";

/**
 * PresetBar applies, saves, renames and deletes presets.
 *
 * Built-ins cannot be renamed or deleted, so their controls simply are not
 * offered rather than being shown disabled.
 */
export function PresetBar({
  presets,
  activeId,
  options,
  onApply,
  onChanged,
}: {
  presets: presetModels.Preset[] | null;
  activeId: string | null;
  options: core.Options;
  onApply: (preset: presetModels.Preset) => void;
  onChanged: () => void;
}) {
  const [saving, setSaving] = useState(false);
  const [name, setName] = useState("");
  const [error, setError] = useState("");
  const [renaming, setRenaming] = useState<string | null>(null);

  async function save() {
    setError("");
    try {
      await api.SavePreset(name, options);
      setName("");
      setSaving(false);
      onChanged();
    } catch (e) {
      setError(String(e));
    }
  }

  async function rename(id: string, next: string) {
    setError("");
    try {
      await api.RenamePreset(id, next);
      setRenaming(null);
      onChanged();
    } catch (e) {
      setError(String(e));
    }
  }

  if (!presets) {
    return <div className="h-7" aria-hidden />;
  }

  return (
    <div className="flex flex-col gap-xs">
      <div className="flex items-center justify-between gap-sm">
        <Eyebrow>Presets</Eyebrow>
        {!saving && (
          <Button variant="tertiary" size="sm" onClick={() => setSaving(true)}>
            Save current
          </Button>
        )}
      </div>

      {presets.length === 0 ? (
        <EmptyState
          title="No presets yet"
          description="Set up the options you want, then save them here to reuse later."
          className="py-lg"
        />
      ) : (
        <div className="flex flex-wrap items-center gap-xxs">
          {presets.map((preset) =>
            renaming === preset.id ? (
              <RenameField
                key={preset.id}
                initial={preset.name}
                onCancel={() => setRenaming(null)}
                onSubmit={(next) => rename(preset.id, next)}
              />
            ) : (
              <span key={preset.id} className="group relative inline-flex items-center">
                <Chip
                  selected={activeId === preset.id}
                  onClick={() => onApply(preset)}
                  onDoubleClick={() => !preset.builtIn && setRenaming(preset.id)}
                  title={preset.builtIn ? preset.name : `${preset.name} — double-click to rename`}
                  className={cx(!preset.builtIn && "pr-6")}
                >
                  {preset.name}
                </Chip>
                {!preset.builtIn && (
                  <button
                    type="button"
                    aria-label={`Delete ${preset.name}`}
                    onClick={async () => {
                      await api.DeletePreset(preset.id);
                      onChanged();
                    }}
                    className={cx(
                      "no-drag absolute right-1.5 text-caption text-ink-tertiary",
                      "opacity-0 transition-opacity group-hover:opacity-100 hover:text-danger-strong",
                    )}
                  >
                    ×
                  </button>
                )}
              </span>
            ),
          )}
        </div>
      )}

      {saving && (
        <div className="flex items-center gap-xs">
          <Input
            autoFocus
            value={name}
            placeholder="Preset name"
            onChange={(e) => setName(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") void save();
              if (e.key === "Escape") {
                e.stopPropagation();
                setSaving(false);
              }
            }}
            className="flex-1"
          />
          <Button variant="primary" size="sm" onClick={save} disabled={!name.trim()}>
            Save
          </Button>
          <Button variant="tertiary" size="sm" onClick={() => setSaving(false)}>
            Cancel
          </Button>
        </div>
      )}

      {error && <p className="text-caption text-danger-strong">{error}</p>}
    </div>
  );
}

function RenameField({
  initial,
  onSubmit,
  onCancel,
}: {
  initial: string;
  onSubmit: (name: string) => void;
  onCancel: () => void;
}) {
  const [value, setValue] = useState(initial);

  return (
    <Input
      autoFocus
      value={value}
      onChange={(e) => setValue(e.target.value)}
      onBlur={onCancel}
      onKeyDown={(e) => {
        if (e.key === "Enter") onSubmit(value);
        if (e.key === "Escape") {
          e.stopPropagation();
          onCancel();
        }
      }}
      className="w-40"
    />
  );
}
