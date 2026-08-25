import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { Loader2 } from 'lucide-react';
import { mileageSchema, type MileageFormInput } from '../../../../lib/schemas/mileage';
import { Button } from '../../../ui/button';
import { Input } from '../../../ui/input';
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from '../../../ui/form';

interface MileageFormProps {
  defaultMileage?: number;
  onSubmit: (values: MileageFormInput) => Promise<void> | void;
  onCancel?: () => void;
  submitting?: boolean;
}

/**
 * Form for logging a manual mileage record.
 * defaultMileage is pre-filled from the latest record (auto-fill feature).
 */
export function MileageForm({ defaultMileage, onSubmit, onCancel, submitting }: MileageFormProps) {
  const form = useForm<MileageFormInput>({
    resolver: zodResolver(mileageSchema),
    defaultValues: {
      mileage: defaultMileage,
    },
  });

  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit((values) => onSubmit(values))} className="space-y-4">
        <FormField
          control={form.control}
          name="mileage"
          render={({ field }) => (
            <FormItem required>
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

        <div className="flex justify-end gap-2">
          {onCancel && (
            <Button type="button" variant="outline" onClick={onCancel}>
              Cancel
            </Button>
          )}
          <Button type="submit" disabled={submitting}>
            {submitting && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            Log Mileage
          </Button>
        </div>
      </form>
    </Form>
  );
}
