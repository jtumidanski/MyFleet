import { useForm, useWatch } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { Loader2 } from 'lucide-react';
import {
  maintenanceScheduleSchema,
  type MaintenanceScheduleFormInput,
} from '../../../../lib/schemas/maintenanceSchedule';
import { Button } from '../../../ui/button';
import { Input } from '../../../ui/input';
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from '../../../ui/form';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '../../../ui/select';
import { CategoryCombobox } from '../CategoryCombobox';
import { RequiredLegend } from '../../../ui/required';
import type { MaintenanceCategory } from '../../../../types/models/maintenanceCategory';

interface MaintenanceScheduleFormProps {
  categories: MaintenanceCategory[];
  onSubmit: (values: MaintenanceScheduleFormInput) => Promise<void> | void;
  onCancel?: () => void;
  submitting?: boolean;
}

/**
 * Form for defining a maintenance schedule.
 *
 * Recurrence type conditionally shows interval fields:
 *  - time:    intervalMonths only
 *  - mileage: intervalMiles only
 *  - hybrid:  both intervalMonths and intervalMiles
 */
export function MaintenanceScheduleForm({
  categories,
  onSubmit,
  onCancel,
  submitting,
}: MaintenanceScheduleFormProps) {
  const form = useForm<MaintenanceScheduleFormInput>({
    resolver: zodResolver(maintenanceScheduleSchema),
    defaultValues: {
      categoryId: '',
      recurrenceType: 'time',
      intervalMonths: undefined,
      intervalMiles: undefined,
    },
  });

  // Watch recurrenceType to conditionally show interval fields.
  const recurrenceType = useWatch({ control: form.control, name: 'recurrenceType' });

  const showMonths = recurrenceType === 'time' || recurrenceType === 'hybrid';
  const showMiles = recurrenceType === 'mileage' || recurrenceType === 'hybrid';

  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit((values) => onSubmit(values))} className="space-y-4">
        <FormField
          control={form.control}
          name="categoryId"
          render={({ field }) => (
            <FormItem required>
              <FormLabel>Category</FormLabel>
              <FormControl>
                <CategoryCombobox
                  categories={categories}
                  kind="maintenance"
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
          name="recurrenceType"
          render={({ field }) => (
            <FormItem required>
              <FormLabel>Recurrence Type</FormLabel>
              <Select onValueChange={field.onChange} value={field.value}>
                <FormControl>
                  <SelectTrigger>
                    <SelectValue placeholder="Select recurrence type" />
                  </SelectTrigger>
                </FormControl>
                <SelectContent>
                  <SelectItem value="time">Time-based</SelectItem>
                  <SelectItem value="mileage">Mileage-based</SelectItem>
                  <SelectItem value="hybrid">Hybrid (time + mileage)</SelectItem>
                </SelectContent>
              </Select>
              <FormMessage />
            </FormItem>
          )}
        />

        {showMonths && (
          <FormField
            control={form.control}
            name="intervalMonths"
            render={({ field }) => (
              /* Required for `time` and `hybrid` — the same rule the schema's
                 superRefine enforces (lib/schemas/maintenanceSchedule.ts:5-12)
                 and the same boolean that decides whether this field renders,
                 so schema, visibility and marker cannot drift apart. */
              <FormItem required={showMonths}>
                <FormLabel>Every (months)</FormLabel>
                <FormControl>
                  <Input
                    type="number"
                    min={1}
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
        )}

        {showMiles && (
          <FormField
            control={form.control}
            name="intervalMiles"
            render={({ field }) => (
              /* Required for `mileage` and `hybrid` — same rule, same boolean
                 as the field's visibility. */
              <FormItem required={showMiles}>
                <FormLabel>Every (miles)</FormLabel>
                <FormControl>
                  <Input
                    type="number"
                    min={1}
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
        )}

        <RequiredLegend />

        <div className="flex justify-end gap-2">
          {onCancel && (
            <Button type="button" variant="outline" onClick={onCancel}>
              Cancel
            </Button>
          )}
          <Button type="submit" disabled={submitting}>
            {submitting && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            Save Schedule
          </Button>
        </div>
      </form>
    </Form>
  );
}
