/**
 * InviteForm — create a new fleet invite (owner-only). RHF + Zod.
 */
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { Loader2 } from 'lucide-react';
import { toast } from 'sonner';
import { createErrorFromUnknown } from '@myfleet/shared-ts';
import { createInviteSchema, type CreateInviteInput } from '../../../lib/schemas/fleetSettings';
import { Button } from '../../ui/button';
import { Input } from '../../ui/input';
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from '../../ui/form';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '../../ui/select';
import { useCreateInvite } from '../../../lib/hooks/api/invites';

interface InviteFormProps {
  fleetId: string;
}

export function InviteForm({ fleetId }: InviteFormProps) {
  const createInvite = useCreateInvite(fleetId);

  const form = useForm<CreateInviteInput>({
    resolver: zodResolver(createInviteSchema),
    defaultValues: { email: '', role: 'member' },
  });

  const onSubmit = async (values: CreateInviteInput) => {
    try {
      await createInvite.mutateAsync(values);
      // Not "sent": nothing delivers invites yet, and saying so left owners
      // waiting on an email that was never going to arrive. The invite is a row
      // until someone copies its link out of the list below.
      toast.success(`Invite created for ${values.email} — copy its link below to send it`);
      form.reset({ email: '', role: 'member' });
    } catch (err) {
      const apiError = createErrorFromUnknown(err);
      toast.error(apiError.message || 'Could not create invite');
    }
  };

  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4 max-w-sm">
        <FormField
          control={form.control}
          name="email"
          render={({ field }) => (
            <FormItem>
              <FormLabel>Email</FormLabel>
              <FormControl>
                <Input type="email" placeholder="user@example.com" {...field} />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name="role"
          render={({ field }) => (
            <FormItem>
              <FormLabel>Role</FormLabel>
              <Select onValueChange={field.onChange} defaultValue={field.value}>
                <FormControl>
                  <SelectTrigger>
                    <SelectValue placeholder="Select a role" />
                  </SelectTrigger>
                </FormControl>
                <SelectContent>
                  <SelectItem value="member">Member</SelectItem>
                  <SelectItem value="viewer">Viewer</SelectItem>
                </SelectContent>
              </Select>
              <FormMessage />
            </FormItem>
          )}
        />
        <Button type="submit" disabled={createInvite.isPending}>
          {createInvite.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
          Send Invite
        </Button>
      </form>
    </Form>
  );
}
