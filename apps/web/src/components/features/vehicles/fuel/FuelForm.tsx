import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { Loader2 } from 'lucide-react';
import { fuelSchema, type FuelFormInput } from '../../../../lib/schemas/fuel';
import { Button } from '../../../ui/button';
import { Input } from '../../../ui/input';
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from '../../../ui/form';

interface FuelFormProps {
  /** Pre-filled from latest mileage record (auto-fill). */
  defaultMileage?: number;
  /**
   * Prefills the form for editing an existing fuel log. Omitted fields fall
   * back to the create-flow defaults below (now / blank / `defaultMileage`).
   * Merged in last, same pattern as `MaintenanceRecordForm`.
   */
  defaultValues?: Partial<FuelFormInput>;
  onSubmit: (values: FuelFormInput) => Promise<void> | void;
  onCancel?: () => void;
  submitting?: boolean;
}

/**
 * Form for logging a fuel entry.
 *
 * - Mileage pre-filled from latest mileage record, or overridden by `defaultValues`.
 * - Either pricePerGallon or totalCost must be provided (server derives the missing one, §10.5).
 */
export function FuelForm({
  defaultMileage,
  defaultValues,
  onSubmit,
  onCancel,
  submitting,
}: FuelFormProps) {
  const now = new Date().toISOString().slice(0, 16); // YYYY-MM-DDTHH:MM for datetime-local

  const form = useForm<FuelFormInput>({
    resolver: zodResolver(fuelSchema),
    defaultValues: {
      date: now,
      mileage: defaultMileage,
      gallons: undefined,
      totalCost: undefined,
      pricePerGallon: undefined,
      ...defaultValues,
    },
  });

  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit((values) => onSubmit(values))} className="space-y-4">
        <FormField
          control={form.control}
          name="date"
          render={({ field }) => (
            <FormItem>
              <FormLabel>Date</FormLabel>
              <FormControl>
                <Input type="datetime-local" {...field} />
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
            name="gallons"
            render={({ field }) => (
              <FormItem>
                <FormLabel>Gallons</FormLabel>
                <FormControl>
                  <Input
                    type="number"
                    step="0.001"
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

        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <FormField
            control={form.control}
            name="totalCost"
            render={({ field }) => (
              <FormItem>
                <FormLabel>Total Cost ($)</FormLabel>
                <FormControl>
                  <Input
                    type="number"
                    step="0.01"
                    placeholder="e.g. 52.40"
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
            name="pricePerGallon"
            render={({ field }) => (
              <FormItem>
                <FormLabel>Price per Gallon ($)</FormLabel>
                <FormControl>
                  <Input
                    type="number"
                    step="0.001"
                    placeholder="e.g. 3.999"
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

        <p className="text-xs text-muted-foreground">
          Provide total cost, price per gallon, or both — the server derives the missing value.
        </p>

        <div className="flex justify-end gap-2">
          {onCancel && (
            <Button type="button" variant="outline" onClick={onCancel}>
              Cancel
            </Button>
          )}
          <Button type="submit" disabled={submitting}>
            {submitting && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            Log Fill-up
          </Button>
        </div>
      </form>
    </Form>
  );
}
