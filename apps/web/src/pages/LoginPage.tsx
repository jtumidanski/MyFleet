import { useEffect, useState } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { Loader2 } from 'lucide-react';
import { useAuth } from '../context/AuthContext';
import { useTheme } from '../context/ThemeContext';
import { BrandMark } from '../components/BrandMark';
import { GoogleMark } from '../components/GoogleMark';
import { ThemeToggleButton } from '../components/ThemeToggleButton';
import { Button } from '../components/ui/button';
import { consumeLoginError, noticeFor } from '../lib/auth/loginError';

/**
 * The only screen an unauthenticated visitor sees. Three states: ready,
 * redirecting, and failed.
 *
 * The button performs a full navigation to auth-service's
 * `GET /api/auth/login/google`, which runs the OAuth dance and redirects back
 * either with `#access_token=<jwt>` (AuthProvider captures it) or, on any
 * failure, back here with `#error=<code>` (consumeLoginError reads it).
 * Already-authenticated visitors are bounced to the app.
 */
export function LoginPage() {
  const { login, isAuthenticated } = useAuth();
  // No mutation behind the control: there is no session on /login, so the
  // authenticated PATCH would 401 and toast on every click (FR-PRETOGGLE-3).
  const { preference, setPreference } = useTheme();
  const navigate = useNavigate();
  const location = useLocation();
  // Set by RequireAuth when it bounced the user here; handed to auth-service so
  // the OAuth callback returns to the page they actually wanted.
  const from = (location.state as { from?: string } | null)?.from;
  // Read once per page load; the module memoises, so StrictMode's remount does
  // not swallow the notice (design §4.1).
  const [errorCode] = useState(consumeLoginError);
  const [redirecting, setRedirecting] = useState(false);

  useEffect(() => {
    // Honour `from` here too: a visitor who authenticated in another tab and
    // came back would otherwise lose the invite they were bounced off.
    if (isAuthenticated) navigate(from ?? '/', { replace: true });
  }, [isAuthenticated, navigate, from]);

  const notice = errorCode ? noticeFor(errorCode) : null;
  const failed = notice?.tone === 'danger';

  return (
    <div className="relative flex min-h-screen flex-col justify-center bg-background px-6 sm:px-12 lg:px-24">
      <div className="absolute right-4 top-4">
        <ThemeToggleButton preference={preference} onSelect={setPreference} />
      </div>

      <div className="w-full max-w-xl space-y-8">
        <div className="flex items-center gap-2 text-xs font-medium uppercase tracking-[0.2em] text-muted-foreground">
          <BrandMark className="h-4 w-4" />
          MyFleet
        </div>

        {/* Capped near 11ch so it wraps to two lines predictably rather than
            overflowing a 320px viewport (FR-PAGE-9). */}
        <h1 className="max-w-[11ch] text-4xl font-semibold leading-[1.05] tracking-tight sm:text-5xl lg:text-6xl">
          <span className="block text-foreground">Your cars.</span>
          <span className="block text-muted-foreground">One place.</span>
        </h1>

        <div className="h-px bg-border" />

        {/* role="alert" only on the danger branch: a cancellation is not an
            alert, and announcing it as one is the same category error as
            painting it red (FR-STATE-4, FR-STATE-5, FR-A11Y-1). */}
        {notice &&
          (failed ? (
            <div
              role="alert"
              className="rounded-md border border-danger-border bg-danger-subtle p-3 text-sm text-danger-subtle-foreground"
            >
              {notice.message}
            </div>
          ) : (
            <p className="text-sm text-muted-foreground">{notice.message}</p>
          ))}

        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:gap-6">
          <Button
            type="button"
            className="w-full sm:w-auto"
            disabled={redirecting}
            onClick={() => {
              // One-way: login() is a full navigation, so this state has no
              // exit path in-page (FR-STATE-2) and `disabled` makes a second
              // activation impossible (FR-STATE-3).
              setRedirecting(true);
              login(from);
            }}
          >
            {redirecting ? (
              <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" />
            ) : (
              <GoogleMark className="h-4 w-4" />
            )}
            {redirecting ? 'Redirecting to Google…' : failed ? 'Try again' : 'Continue with Google'}
          </Button>
          <p className="text-sm text-muted-foreground">
            MyFleet receives your name, email address, and profile photo.
          </p>
        </div>

        {/* A live region, because relabelling an element that has just become
            disabled is not reliably announced (FR-A11Y-2).

            Rendered UNCONDITIONALLY, with only its text toggling. A region
            mounted together with its text is a region that did not exist when
            the announcement was made: assistive tech watches for changes
            *inside* live regions it already knows about, and treats a
            freshly-inserted one inconsistently. Compounding it, `disabled` on
            the button drops focus to <body> at that same instant, so there is
            no focused element to anchor the announcement to either. Keeping the
            empty region in the tree from first paint means the press is a
            content change in a region already being observed. */}
        <span className="sr-only" role="status">
          {redirecting ? 'Redirecting to Google…' : ''}
        </span>

        <p className="text-sm text-muted-foreground">
          Maintenance, mileage, and receipts for every car in your household. Sign in with Google.
        </p>
      </div>
    </div>
  );
}
