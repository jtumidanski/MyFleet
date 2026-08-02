import { useEffect } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { useAuth } from '../context/AuthContext';
import { Button } from '../components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../components/ui/card';

/**
 * Google sign-in landing page. The button performs a full navigation to
 * auth-service's `GET /api/auth/login/google`, which runs the OAuth dance and
 * redirects back to the SPA with the access token in the URL fragment.
 * Already-authenticated visitors are bounced to the app.
 */
export function LoginPage() {
  const { login, isAuthenticated } = useAuth();
  const navigate = useNavigate();
  const location = useLocation();
  // Set by RequireAuth when it bounced the user here; handed to auth-service so
  // the OAuth callback returns to the page they actually wanted.
  const from = (location.state as { from?: string } | null)?.from;

  useEffect(() => {
    if (isAuthenticated) navigate('/', { replace: true });
  }, [isAuthenticated, navigate]);

  return (
    <div className="flex min-h-screen items-center justify-center bg-muted">
      <Card className="w-full max-w-sm">
        <CardHeader className="items-center text-center">
          <CardTitle>MyFleet</CardTitle>
          <CardDescription>Sign in to manage your household fleet.</CardDescription>
        </CardHeader>
        <CardContent>
          <Button type="button" variant="outline" className="w-full" onClick={() => login(from)}>
            Continue with Google
          </Button>
        </CardContent>
      </Card>
    </div>
  );
}
