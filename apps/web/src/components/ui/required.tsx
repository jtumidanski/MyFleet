/**
 * The `*` on a required field's label. Decorative by construction: the
 * semantic signal is `aria-required` on the control, which FormControl emits
 * from the same FormItem `required` flag. Announcing "Make star" would be a
 * regression, and an asterisk in the label's own text node would break the
 * exact-string label assertions in MaintenanceRecordForm.test.tsx.
 *
 * text-danger, not text-destructive: --destructive is ~2:1 against the dark
 * background, while --danger measures 6.67:1 light / 7.23:1 dark
 * (docs/tasks/task-003-dark-mode-branding/contrast.md:18,27). index.css:104-108
 * records the split — the bare status tokens are for text on --background /
 * --card, --destructive styles destructive controls.
 */
export function RequiredMarker() {
  // Leading space inside the span, on one line, so JSX cannot trim it: the
  // rendered text must be "Make *", never "Make*".
  return <span aria-hidden="true" className="text-danger"> *</span>;
}

/**
 * `* Required` legend. Rendered at the bottom of the field stack on forms with
 * three or more fields (FR-10/FR-11); shorter forms are self-explanatory.
 *
 * Its asterisk is the subject of the sentence rather than decoration, so —
 * unlike RequiredMarker — it is not aria-hidden. It carries the same
 * text-danger so both glyphs read as the same mark.
 */
export function RequiredLegend() {
  return (
    <p className="text-sm text-muted-foreground">
      <span className="text-danger">*</span> Required
    </p>
  );
}
