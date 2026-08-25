import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { toast } from 'sonner';
import { Loader2 } from 'lucide-react';
import { createErrorFromUnknown } from '@myfleet/shared-ts';
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '../../../ui/dialog';
import { Button } from '../../../ui/button';
import { Input } from '../../../ui/input';
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from '../../../ui/form';
import { useCompleteMaintenanceSchedule } from '../../../../lib/hooks/api/maintenance';
import { useMileageRecords, getLatestMileage } from '../../../../lib/hooks/api/mileage';
import { useVehicle } from '../../../../lib/hooks/api/vehicles';
import {
  completeScheduleSchema,
  type CompleteScheduleFormInput,
} from '../../../../lib/schemas/maintenanceSchedule';
import type { MaintenanceSchedule } from '../../../../types/models/maintenanceSchedule';

interface CompleteScheduleDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  schedule: MaintenanceSchedule;
}

/**
 * No existing form covers "mark a schedule complete" — this is a small
 * react-hook-form + Zod form built for the dialog, on the same pattern as its
 * siblings. Handler mirrors VehicleMaintenanceSection.tsx:126-143, except the
 * date and odometer are now user-editable rather than always "now".
 */
export function CompleteScheduleDialog({
  open,
  onOpenChange,
  schedule,
}: CompleteScheduleDialogProps) {
  const vehicleId = schedule.attributes.vehicleId;
  const { data: vehicle } = useVehicle(vehicleId);
  const { data: mileageData } = useMileageRecords({ vehicleId });
  const completeSchedule = useCompleteMaintenanceSchedule(vehicleId);

  const latestLogged = getLatestMileage(mileageData?.rows ?? []);
  const autoFillMileage = latestLogged ?? vehicle?.attributes.currentMileage;

  const today = new Date().toISOString().slice(0, 10); // YYYY-MM-DD

  const form = useForm<CompleteScheduleFormInput>({
    resolver: zodResolver(completeScheduleSchema),
    values: {
      date: today,
      latestMileage: autoFillMileage,
    },
  });

  const handleSubmit = async (values: CompleteScheduleFormInput) => {
    try {
      await completeSchedule.mutateAsync({
        id: schedule.id,
        attributes: {
          date: new Date(values.date).toISOString(),
          latestMileage: values.latestMileage,
        },
      });
      toast.success('Maintenance marked as complete');
      onOpenChange(false);
    } catch (err) {
      const apiError = createErrorFromUnknown(err);
      toast.error(apiError.message || 'Could not complete maintenance schedule');
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Mark Schedule Complete</DialogTitle>
        </DialogHeader>
        <Form {...form}>
          <form onSubmit={form.handleSubmit(handleSubmit)} className="space-y-4">
            <FormField
              control={form.control}
              name="date"
              render={({ field }) => (
                <FormItem required>
                  <FormLabel>Date completed</FormLabel>
                  <FormControl>
                    <Input type="date" {...field} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name="latestMileage"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Odometer (miles)</FormLabel>
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

            <div className="flex justify-end gap-2">
              <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
                Cancel
              </Button>
              <Button type="submit" disabled={completeSchedule.isPending}>
                {completeSchedule.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                Mark Complete
              </Button>
            </div>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}
