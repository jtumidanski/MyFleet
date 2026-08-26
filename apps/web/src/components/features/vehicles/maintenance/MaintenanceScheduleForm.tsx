import { useEffect, useRef } from 'react';
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
  /** The vehicle's odometer, used to default a recurring schedule's first-due odometer. */
  currentMileage?: number;
}

/** YYYY-MM-DD, the value format a native date input uses. */
function toDateInputValue(d: Date): string {
  return d.toISOString().slice(0, 10);
}

/**
 * Form for defining a maintenance schedule.
 *
 * Two independent choices. `kind` says whether the schedule repeats;
 * `recurrenceType` says which axes it is judged on — time, mileage, or both —
 * which is the same question for a one-off as for a repeating item, so the
 * select stays visible for both kinds. The field set below it swaps:
 *  - recurring: intervals PLUS a first-due anchor on each covered axis
 *  - one-time:  a due date / due odometer on each covered axis, no intervals
 */
export function MaintenanceScheduleForm({
  categories,
  onSubmit,
  onCancel,
  submitting,
  currentMileage,
}: MaintenanceScheduleFormProps) {
  const form = useForm<MaintenanceScheduleFormInput>({
    resolver: zodResolver(maintenanceScheduleSchema),
    defaultValues: {
      categoryId: '',
      kind: 'recurring',
      recurrenceType: 'time',
      intervalMonths: undefined,
      intervalMiles: undefined,
      dueDate: undefined,
      dueMileage: undefined,
    },
  });

  const kind = useWatch({ control: form.control, name: 'kind' });
  const recurrenceType = useWatch({ control: form.control, name: 'recurrenceType' });
  const intervalMonths = useWatch({ control: form.control, name: 'intervalMonths' });
  const intervalMiles = useWatch({ control: form.control, name: 'intervalMiles' });

  const recurring = kind === 'recurring';
  const showMonths = recurrenceType === 'time' || recurrenceType === 'hybrid';
  const showMiles = recurrenceType === 'mileage' || recurrenceType === 'hybrid';

  const { setValue } = form;

  // FR-ANCHOR-4: a user who does not care about the anchor gets the intuitive
  // "starts now" behaviour for free. The guard that keeps a later interval
  // edit from stomping an anchor the user chose deliberately can't be RHF's
  // `dirtyFields` — that flag is a diff against defaultValues, so a value
  // *this effect* sets away from `undefined` registers as "dirty" too, which
  // freezes the default after its very first write. These refs track only
  // edits that flowed through the field's own onChange (i.e. the user typed
  // in it), so the effect keeps recomputing until an actual user edit happens.
  const dueDateTouched = useRef(false);
  const dueMileageTouched = useRef(false);

  useEffect(() => {
    if (!recurring) return;
    if (showMonths && intervalMonths && !dueDateTouched.current) {
      const next = new Date();
      next.setMonth(next.getMonth() + intervalMonths);
      setValue('dueDate', toDateInputValue(next));
    }
    if (showMiles && intervalMiles && currentMileage !== undefined && !dueMileageTouched.current) {
      setValue('dueMileage', currentMileage + intervalMiles);
    }
  }, [recurring, showMonths, showMiles, intervalMonths, intervalMiles, currentMileage, setValue]);

  // A one-time schedule forbids intervalMonths/intervalMiles (see the schema's
  // superRefine), but the interval FormFields only render while `recurring` is
  // true. Without this, switching to one-time leaves a stale interval value in
  // form state that the resolver rejects with no visible field to show the
  // error on — the form silently refuses to submit. Clearing on the way out is
  // enough: those fields stay unmounted for the rest of the one-time session,
  // so nothing repopulates them before a possible switch back to recurring.
  useEffect(() => {
    if (recurring) return;
    setValue('intervalMonths', undefined);
    setValue('intervalMiles', undefined);
  }, [recurring, setValue]);

  const dueDateLabel = recurring ? 'First due date' : 'Due date';
  const dueMileageLabel = recurring ? 'First due odometer (miles)' : 'Due odometer (miles)';

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
          name="kind"
          render={({ field }) => (
            <FormItem required>
              <FormLabel>Schedule Type</FormLabel>
              <Select onValueChange={field.onChange} value={field.value}>
                <FormControl>
                  <SelectTrigger>
                    <SelectValue placeholder="Select schedule type" />
                  </SelectTrigger>
                </FormControl>
                <SelectContent>
                  <SelectItem value="recurring">Repeating</SelectItem>
                  <SelectItem value="oneTime">One-time</SelectItem>
                </SelectContent>
              </Select>
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

        {recurring && showMonths && (
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

        {recurring && showMiles && (
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

        {showMonths && (
          <FormField
            control={form.control}
            name="dueDate"
            render={({ field }) => (
              /* Required whenever the schedule covers time — the same boolean
                 that decides whether the field renders at all. */
              <FormItem required={showMonths}>
                <FormLabel>{dueDateLabel}</FormLabel>
                <FormControl>
                  <Input
                    type="date"
                    value={field.value ?? ''}
                    onChange={(e) => {
                      dueDateTouched.current = true;
                      field.onChange(e);
                    }}
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
            name="dueMileage"
            render={({ field }) => (
              /* Required whenever the schedule covers mileage — same boolean
                 as the field's visibility. */
              <FormItem required={showMiles}>
                <FormLabel>{dueMileageLabel}</FormLabel>
                <FormControl>
                  <Input
                    type="number"
                    min={1}
                    value={field.value ?? ''}
                    onChange={(e) => {
                      dueMileageTouched.current = true;
                      field.onChange(e.target.value === '' ? undefined : e.target.valueAsNumber);
                    }}
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
