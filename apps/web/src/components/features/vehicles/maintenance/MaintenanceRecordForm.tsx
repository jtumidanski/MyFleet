import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { Loader2 } from 'lucide-react';
import {
  maintenanceRecordSchema,
  type MaintenanceRecordFormInput,
} from '../../../../lib/schemas/maintenanceRecord';
import { usePendingAttachments } from '../../../../lib/hooks/usePendingAttachments';
import { Button } from '../../../ui/button';
import { Input } from '../../../ui/input';
import { Textarea } from '../../../ui/textarea';
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from '../../../ui/form';
import { CategoryCombobox } from '../CategoryCombobox';
import { AttachmentPicker } from './AttachmentPicker';
import type {
  MaintenanceCategory,
  MaintenanceCategoryKind,
} from '../../../../types/models/maintenanceCategory';

interface MaintenanceRecordFormProps {
  categories: MaintenanceCategory[];
  defaultMileage?: number;
  /**
   * Restricts the category picker to one kind and relabels the submit button.
   * The picker is not grouped by kind because it never shows more than one kind
   * at a time (design D19).
   *
   * Required: `CategoryCombobox` assigns this kind to anything created inline,
   * so an unset kind here would silently mis-kind a newly created category
   * (it would still pass a value to the server, just the wrong one).
   */
  kind: MaintenanceCategoryKind;
  /**
   * Prefills the form for editing an existing record. Omitted fields fall
   * back to the create-flow defaults below (blank / `defaultMileage`).
   * Merged in last, so an explicit `undefined` on a field here still yields
   * that field's create-flow default rather than `undefined` itself.
   */
  defaultValues?: Partial<MaintenanceRecordFormInput>;
  onSubmit: (
    values: MaintenanceRecordFormInput,
    documentMediaIds: string[],
  ) => Promise<void> | void;
  onCancel?: () => void;
  submitting?: boolean;
  /**
   * Attachments the record already holds, on the edit path. Threaded to both
   * the pending-upload hook and the picker so the ten-per-record cap counts
   * existing plus pending rather than pending alone. Defaults to 0 for the
   * create flow, where there is no record yet.
   */
  existingAttachmentCount?: number;
}

/**
 * Form for logging a maintenance record.
 * - Category dropdown populated from GET /maintenance-categories, filtered by `kind`.
 * - Mileage pre-filled from latest mileage record (auto-fill), or overridden by `defaultValues`.
 */
export function MaintenanceRecordForm({
  categories,
  defaultMileage,
  kind,
  defaultValues,
  onSubmit,
  onCancel,
  submitting,
  existingAttachmentCount = 0,
}: MaintenanceRecordFormProps) {
  const now = new Date().toISOString().slice(0, 16); // YYYY-MM-DDTHH:MM for datetime-local
  const attachments = usePendingAttachments(existingAttachmentCount);

  const visibleCategories = categories.filter((c) => c.attributes.kind === kind);

  const form = useForm<MaintenanceRecordFormInput>({
    resolver: zodResolver(maintenanceRecordSchema),
    defaultValues: {
      categoryId: '',
      performedAt: now,
      description: '',
      mileage: defaultMileage,
      cost: undefined,
      vendor: '',
      notes: '',
      documentMediaIds: [],
      ...defaultValues,
    },
  });

  return (
    <Form {...form}>
      <form
        onSubmit={form.handleSubmit((values) => onSubmit(values, attachments.commit()))}
        className="space-y-4"
      >
        <FormField
          control={form.control}
          name="categoryId"
          render={({ field }) => (
            <FormItem>
              <FormLabel>Category</FormLabel>
              <FormControl>
                <CategoryCombobox
                  categories={visibleCategories}
                  kind={kind}
                  value={field.value}
                  onChange={field.onChange}
                  ariaLabel="Category"
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />

        <FormField
          control={form.control}
          name="performedAt"
          render={({ field }) => (
            <FormItem>
              <FormLabel>Date Performed</FormLabel>
              <FormControl>
                <Input type="datetime-local" {...field} />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />

        <FormField
          control={form.control}
          name="description"
          render={({ field }) => (
            <FormItem>
              <FormLabel>Description</FormLabel>
              <FormControl>
                <Input
                  type="text"
                  placeholder="Cat-back exhaust, Borla S-Type"
                  {...field}
                  value={field.value ?? ''}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />

        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <FormField
            control={form.control}
            name="mileage"
            render={({ field }) => (
              <FormItem>
                <FormLabel>Mileage (miles)</FormLabel>
                <FormControl>
                  <Input
                    type="number"
                    value={field.value ?? ''}
                    onChange={(e) =>
                      field.onChange(e.target.value === '' ? undefined : e.target.valueAsNumber)
                    }
                    onBlur={field.onBlur}
                    name={field.name}
                    ref={field.ref}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name="cost"
            render={({ field }) => (
              <FormItem>
                <FormLabel>Cost ($)</FormLabel>
                <FormControl>
                  <Input
                    type="number"
                    step="0.01"
                    value={field.value ?? ''}
                    onChange={(e) =>
                      field.onChange(e.target.value === '' ? undefined : e.target.valueAsNumber)
                    }
                    onBlur={field.onBlur}
                    name={field.name}
                    ref={field.ref}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        </div>

        <FormField
          control={form.control}
          name="vendor"
          render={({ field }) => (
            <FormItem>
              <FormLabel>Vendor / Shop</FormLabel>
              <FormControl>
                <Input type="text" {...field} value={field.value ?? ''} />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />

        <FormField
          control={form.control}
          name="notes"
          render={({ field }) => (
            <FormItem>
              <FormLabel>Notes</FormLabel>
              <FormControl>
                <Textarea rows={3} {...field} value={field.value ?? ''} />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />

        <AttachmentPicker
          items={attachments.items}
          onAdd={attachments.add}
          onRemove={attachments.remove}
          existingCount={existingAttachmentCount}
        />

        <div className="flex justify-end gap-2">
          {onCancel && (
            <Button type="button" variant="outline" onClick={onCancel}>
              Cancel
            </Button>
          )}
          <Button type="submit" disabled={submitting || attachments.isUploading}>
            {(submitting || attachments.isUploading) && (
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
            )}
            {kind === 'modification' ? 'Log Modification' : 'Log Record'}
          </Button>
        </div>
      </form>
    </Form>
  );
}
