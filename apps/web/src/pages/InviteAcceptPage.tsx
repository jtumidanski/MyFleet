/**
 * InviteAcceptPage — accepts a fleet invite via token (Task 15.8).
 * Route: /invites/:token/accept
 *
 * POST /api/fleet/invites/{token}/accept — no body needed.
 * On success: shows confirmation and redirects to home.
 * On error: shows the error message (e.g. expired, already accepted, email mismatch).
 */
import { useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { Loader2, CheckCircle, XCircle } from 'lucide-react';
import { createErrorFromUnknown } from '@myfleet/shared-ts';
import { Button } from '../components/ui/button';
import { useAcceptInvite } from '../lib/hooks/api/invites';

export function InviteAcceptPage() {
  const { token } = useParams<{ token: string }>();
  const navigate = useNavigate();
  const acceptInvite = useAcceptInvite();
  const [status, setStatus] = useState<'pending' | 'success' | 'error'>('pending');
  const [errorMessage, setErrorMessage] = useState<string>('');

  useEffect(() => {
    if (!token) return;
    acceptInvite.mutate(token, {
      onSuccess: () => {
        setStatus('success');
        setTimeout(() => navigate('/'), 2000);
      },
      onError: (err) => {
        const apiError = createErrorFromUnknown(err);
        setErrorMessage(apiError.message || 'Could not accept invite');
        setStatus('error');
      },
    });
    // Only run once on mount
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  if (status === 'pending') {
    return (
      <div className="flex flex-col items-center justify-center min-h-[40vh] gap-4">
        <Loader2 className="h-8 w-8 animate-spin text-primary" />
        <p className="text-sm text-muted-foreground">Accepting invite…</p>
      </div>
    );
  }

  if (status === 'success') {
    return (
      <div className="flex flex-col items-center justify-center min-h-[40vh] gap-4">
        <CheckCircle className="h-8 w-8 text-green-600" />
        <p className="text-base font-medium">You have joined the fleet!</p>
        <p className="text-sm text-muted-foreground">Redirecting to dashboard…</p>
      </div>
    );
  }

  return (
    <div className="flex flex-col items-center justify-center min-h-[40vh] gap-4">
      <XCircle className="h-8 w-8 text-red-600" />
      <p className="text-base font-medium">Could not accept invite</p>
      <p className="text-sm text-muted-foreground">{errorMessage}</p>
      <Button variant="outline" onClick={() => navigate('/')}>
        Go to Dashboard
      </Button>
    </div>
  );
}
