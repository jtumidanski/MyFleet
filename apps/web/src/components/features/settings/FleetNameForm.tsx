/**
 * FleetNameForm — rename fleet (owner-only). RHF + Zod.
 */
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { Loader2 } from 'lucide-react';
import { toast } from 'sonner';
import { createErrorFromUnknown } from '@myfleet/shared-ts';
import { renameFleetSchema, type RenameFleetInput } from '../../../lib/schemas/fleetSettings';
import { Button } from '../../ui/button';
import { Input } from '../../ui/input';
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from '../../ui/form';
import { useRenameFleet } from '../../../lib/hooks/api/fleetSettings';

interface FleetNameFormProps {
  fleetId: string;
  currentName: string;
}

export function FleetNameForm({ fleetId, currentName }: FleetNameFormProps) {
  const rename = useRenameFleet(fleetId);

  const form = useForm<RenameFleetInput>({
    resolver: zodResolver(renameFleetSchema),
    defaultValues: { name: currentName },
  });

  const onSubmit = async (values: RenameFleetInput) => {
    try {
      await rename.mutateAsync(values.name);
      toast.success('Fleet name updated');
      form.reset({ name: values.name });
    } catch (err) {
      const apiError = createErrorFromUnknown(err);
      toast.error(apiError.message || 'Could not rename fleet');
    }
  };

  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4 max-w-sm">
        <FormField
          control={form.control}
          name="name"
          render={({ field }) => (
            <FormItem>
              <FormLabel>Fleet Name</FormLabel>
              <FormControl>
                <Input type="text" {...field} />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <Button type="submit" disabled={rename.isPending || !form.formState.isDirty}>
          {rename.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
          Save
        </Button>
      </form>
    </Form>
  );
}
