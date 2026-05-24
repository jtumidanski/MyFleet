import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { useNavigate } from 'react-router-dom';
import { toast } from 'sonner';
import { Loader2 } from 'lucide-react';
import { createErrorFromUnknown } from '@myfleet/shared-ts';
import { createFleetSchema, type CreateFleetInput } from '../lib/schemas/fleet';
import { useCreateFleet } from '../lib/hooks/api/fleets';
import { Button } from '../components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../components/ui/card';
import { Input } from '../components/ui/input';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '../components/ui/form';

/**
 * First-run onboarding: create the household fleet. On success the access token
 * is refreshed (so it carries the new activeFleetId) and the user lands on the
 * dashboard.
 */
export function OnboardingPage() {
  const navigate = useNavigate();
  const createFleet = useCreateFleet();
  const form = useForm<CreateFleetInput>({
    resolver: zodResolver(createFleetSchema),
    defaultValues: { name: '' },
  });
  const { isSubmitting } = form.formState;

  const onSubmit = form.handleSubmit(async (values) => {
    try {
      await createFleet.mutateAsync(values);
      toast.success('Fleet created');
      navigate('/', { replace: true });
    } catch (err) {
      const apiError = createErrorFromUnknown(err);
      toast.error(apiError.message || 'Could not create fleet');
    }
  });

  return (
    <div className="flex min-h-screen items-center justify-center bg-muted">
      <Card className="w-full max-w-md">
        <CardHeader>
          <CardTitle>Set Up Your Fleet</CardTitle>
          <CardDescription>Give your household fleet a name to get started.</CardDescription>
        </CardHeader>
        <CardContent>
          <Form {...form}>
            <form onSubmit={onSubmit} className="space-y-6">
              <FormField
                control={form.control}
                name="name"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Fleet Name</FormLabel>
                    <FormControl>
                      <Input type="text" autoFocus placeholder="The Smith Household" {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <Button type="submit" className="w-full" disabled={isSubmitting}>
                {isSubmitting && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                Create Fleet
              </Button>
            </form>
          </Form>
        </CardContent>
      </Card>
    </div>
  );
}
