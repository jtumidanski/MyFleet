import { useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { useAuth } from '../context/AuthContext';

/**
 * Google sign-in landing page. The button performs a full navigation to
 * auth-service's `GET /api/auth/login/google`, which runs the OAuth dance and
 * redirects back to the SPA with the access token in the URL fragment.
 * Already-authenticated visitors are bounced to the app.
 */
export function LoginPage() {
  const { login, isAuthenticated } = useAuth();
  const navigate = useNavigate();

  useEffect(() => {
    if (isAuthenticated) navigate('/', { replace: true });
  }, [isAuthenticated, navigate]);

  return (
    <div className="flex min-h-screen items-center justify-center bg-gray-50">
      <div className="w-full max-w-sm rounded-lg border border-gray-200 bg-white p-8 shadow-sm">
        <h1 className="text-center text-2xl font-semibold text-gray-900">MyFleet</h1>
        <p className="mt-2 text-center text-sm text-gray-500">
          Sign in to manage your household fleet.
        </p>
        <button
          type="button"
          onClick={login}
          className="mt-6 flex w-full items-center justify-center gap-2 rounded-md border border-gray-300 bg-white px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50"
        >
          Continue with Google
        </button>
      </div>
    </div>
  );
}
