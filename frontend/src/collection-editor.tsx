import { useEffect, useRef, useState } from "react";
import { Button } from "@/renderer/components/ui/button";
import { TextField, FieldRow, Picker } from "./controls";
import { UnsavedChangesDialog } from "./unsaved-changes-dialog";
import { FormActions } from "./form-actions";
import { useDraftViewModel } from "./viewmodels/use-draft-view-model";
import { collectionDescendants, collectionPath } from "./collections";
import type { Collection } from "./model";
import type { Messages } from "./messages";

export function CollectionEditor({ initial, parent, collections, deleted, m, onSave, onCancel, onDirty, onBusy }: {
  initial?: Collection; parent?: string; collections: Collection[]; deleted?: boolean; m: Messages;
  onSave: (collection: Collection) => Promise<void>; onCancel: () => void;
  onDirty: (dirty: boolean) => void; onBusy: (busy: boolean) => void;
}) {
  const [id] = useState(() => initial?.id ?? crypto.randomUUID());
  const editor = useDraftViewModel({ name: initial?.name ?? "", parent: initial?.parent ?? parent ?? "root" });
  const [saving, setSaving] = useState(false), [error, setError] = useState("");
  const [reload, setReload] = useState(false);
  const reloadButton = useRef<HTMLButtonElement>(null);
  const [submitted, setSubmitted] = useState(false);
  const submitting = useRef(false), nameRef = useRef<HTMLInputElement>(null);
  const excluded = collectionDescendants(id, collections);
  useEffect(() => onDirty(editor.dirty), [editor.dirty, onDirty]);
  useEffect(() => onBusy(saving), [saving, onBusy]);
  return <form className="space-y-4" noValidate inert={saving} aria-busy={saving} onSubmit={async event => {
    event.preventDefault(); setSubmitted(true);
    if (!editor.draft.name.trim()) { nameRef.current?.focus(); return; }
    if (submitting.current || deleted || editor.conflict) return;
    submitting.current = true; setSaving(true); setError("");
    try { await onSave({ id, name: editor.draft.name.trim(), parent: editor.draft.parent === "root" ? null : editor.draft.parent, commandIds: initial?.commandIds ?? [] }); }
    catch (cause) { setError(cause instanceof Error ? cause.message : "storageFailed"); }
    finally { submitting.current = false; setSaving(false); }
  }}>
    <UnsavedChangesDialog open={reload} m={m} onContinue={() => setReload(false)}
      onRestoreFocus={() => reloadButton.current?.focus({ preventScroll: true })}
      onDiscard={() => { editor.reload(); setError(""); setReload(false); }}/>
    <TextField label={m.collectionName} autoFocus required maxLength={200} inputRef={nameRef} value={editor.draft.name}
      error={submitted && !editor.draft.name.trim() ? m.required : undefined} onChange={event => editor.setField("name", event.target.value)}/>
    <FieldRow label={m.parentCollection}><Picker value={editor.draft.parent} label={m.parentCollection}
      options={[["root", m.rootCollection], ...collections.filter(c => !excluded.has(c.id)).map(c => [c.id, collectionPath(c, collections)] as [string, string])]}
      onChange={value => editor.setField("parent", value)}/></FieldRow>
    <FormActions error={deleted ? m.collectionMissing : editor.conflict ? m.conflict : error ? m[error as keyof Messages] ?? m.storageFailed : undefined}>
      {editor.conflict && <Button variant="outline" type="button" ref={reloadButton} onClick={() => setReload(true)}>{m.reload}</Button>}
      <Button type="submit" size="sm" disabled={saving || deleted || editor.conflict}>{saving ? m.saving : m.save}</Button>
      <Button type="button" size="sm" variant="outline" onClick={onCancel}>{m.cancel}</Button>
    </FormActions>
  </form>;
}
