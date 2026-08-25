import { useForm, useWatch } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { toast } from 'sonner';
import { Loader2 } from 'lucide-react';
import { createErrorFromUnknown } from '@myfleet/shared-ts';
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '../../../ui/dialog';
import { Button } from '../../../ui/button';
import { Input } from '../../../ui/input';
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from '../../../ui/form';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '../../../ui/select';
import { useUpdateMaintenanceSchedule } from '../../../../lib/hooks/api/maintenance';
import {
  convertToRecurrenceSchema,
  type ConvertToRecurrenceFormInput,
} from '../../../../lib/schemas/maintenanceSchedule';
import type { MaintenanceSchedule } from '../../../../types/models/maintenanceSchedule';

interface ConvertToRecurrenceDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  schedule: MaintenanceSchedule;
  categoryName: string;
}

/**
 * Turns a just-completed one-time schedule into a recurring one — the
 * "I didn't know this would repeat until I did it" path (FR-CONV-2, FR-CONV-3).
 *
 * Submitting issues exactly ONE PATCH. Next-due is then derived server-side
 * from `lastCompletedDate` / `lastCompletedMileage`, which the completion flow
 * already recorded, so the dialog collects no anchor — only the recurrence type
 * and the intervals it needs.
 *
 * The conversion is strictly additive and never rolls the completion back. If
 * the PATCH fails the dialog stays open with an error toast, and the schedule
 * remains a completed, deactivated one-time schedule — a valid terminal state
 * (FR-CONV-4).
 */
export function ConvertToRecurrenceDialog({
  open,
  onOpenChange,
  schedule,
  categoryName,
}: ConvertToRecurrenceDialogProps) {
  const update = useUpdateMaintenanceSchedule();

  const form = useForm<ConvertToRecurrenceFormInput>({
    resolver: zodResolver(convertToRecurrenceSchema),
    defaultValues: {
      recurrenceType: 'time',
      intervalMonths: undefined,
      intervalMiles: undefined,
    },
  });

  const recurrenceType = useWatch({ control: form.control, name: 'recurrenceType' });
  const showMonths = recurrenceType === 'time' || recurrenceType === 'hybrid';
  const showMiles = recurrenceType === 'mileage' || recurrenceType === 'hybrid';

  const { lastCompletedDate, lastCompletedMileage } = schedule.attributes;
  const anchorParts = [
    lastCompletedDate ? new Date(lastCompletedDate).toLocaleDateString() : null,
    lastCompletedMileage ? `${lastCompletedMileage.toLocaleString()} miles` : null,
  ].filter(Boolean);

  const handleSubmit = async (values: ConvertToRecurrenceFormInput) => {
    try {
      await update.mutateAsync({
        id: schedule.id,
        attributes: {
          oneTime: false,
          recurrenceType: values.recurrenceType,
          intervalMonths: values.intervalMonths ?? 0,
          intervalMiles: values.intervalMiles ?? 0,
          active: true,
          // An explicit null, not an omitted key. The server distinguishes the
          // two (server.Nullable), and omitting it would leave the converted
          // schedule pinned to the one-time due date it just completed.
          dueDate: null,
          dueMileage: 0,
        },
      });
      toast.success('Recurrence set up');
      onOpenChange(false);
    } catch (err) {
      const apiError = createErrorFromUnknown(err);
      toast.error(apiError.message || 'Could not set up recurrence');
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Set Up Recurrence</DialogTitle>
        </DialogHeader>

        <div className="space-y-1">
          <p className="text-sm font-medium">{categoryName}</p>
          {anchorParts.length > 0 && (
            <p className="text-xs text-muted-foreground">
              Repeats from the completion you just recorded: {anchorParts.join(' · ')}
            </p>
          )}
        </div>

        <Form {...form}>
          <form onSubmit={form.handleSubmit(handleSubmit)} className="space-y-4">
            <FormField
              control={form.control}
              name="recurrenceType"
              render={({ field }) => (
                <FormItem>
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
                  <FormItem>
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
                  <FormItem>
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

            <div className="flex justify-end gap-2">
              <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
                Cancel
              </Button>
              <Button type="submit" disabled={update.isPending}>
                {update.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                Set up recurrence
              </Button>
            </div>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}
