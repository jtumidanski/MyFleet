import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { useNavigate } from 'react-router-dom';
import { toast } from 'sonner';
import { Loader2 } from 'lucide-react';
import { createErrorFromUnknown } from '@myfleet/shared-ts';
import { createFleetSchema, type CreateFleetInput } from '../lib/schemas/fleet';
import { useCreateFleet } from '../lib/hooks/api/fleets';
import { usePendingInvites, useAcceptInvite } from '../lib/hooks/api/invites';
import { Button } from '../components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../components/ui/card';
import { Input } from '../components/ui/input';
import { SignedInFooter } from '../components/auth/SignedInFooter';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '../components/ui/form';

/**
 * Any invites waiting for the signed-in user, each acceptable in one click.
 *
 * Renders nothing when there are none, so the page is unchanged for a genuine
 * first-run user. When there IS one, it comes first: someone invited to an
 * existing fleet who is shown only "create a fleet" will create a second, empty
 * one — which is what happened before this section existed, because nothing
 * delivers invites and the accept link never reached them.
 */
function PendingInvites() {
  const navigate = useNavigate();
  const { data: invites } = usePendingInvites();
  const acceptInvite = useAcceptInvite();

  if (!invites || invites.length === 0) return null;

  const accept = (token: string) => {
    acceptInvite.mutate(token, {
      // The mutation does not settle until the refreshed session reports the
      // new membership, so by here the guard will let the dashboard render.
      onSuccess: () => navigate('/', { replace: true }),
    });
  };

  return (
    <Card className="w-full max-w-md">
      <CardHeader>
        <CardTitle>You have an invitation</CardTitle>
        <CardDescription>Join an existing fleet instead of starting your own.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        {invites.map((inv) => (
          <div key={inv.id} className="flex items-center justify-between gap-3">
            <div className="min-w-0 text-sm">
              <div className="truncate font-medium">{inv.attributes.fleetName ?? 'A fleet'}</div>
              <div className="text-xs capitalize text-muted-foreground">{inv.attributes.role}</div>
            </div>
            <Button
              type="button"
              disabled={acceptInvite.isPending}
              onClick={() => accept(inv.attributes.token)}
            >
              {acceptInvite.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              Accept invite
            </Button>
          </div>
        ))}
      </CardContent>
    </Card>
  );
}

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
    <div className="flex min-h-screen flex-col items-center justify-center gap-4 bg-muted py-8">
      <PendingInvites />
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
                  <FormItem required>
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
      {/* Outside PendingInvites, which returns null when nothing is waiting —
          so FR-ONBOARD-2 (it renders on the first-run path too) falls out
          structurally rather than needing a condition. The column already has
          gap-4 and items-center, so no layout change is needed. */}
      <SignedInFooter />
    </div>
  );
}
