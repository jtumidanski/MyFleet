import { useAuth } from '../context/AuthContext';

// Placeholder login — replaced with the full Google sign-in page in Task 14.2.
export function LoginPage() {
  const { login } = useAuth();
  return (
    <div className="flex min-h-screen items-center justify-center">
      <button type="button" onClick={login} className="rounded bg-gray-900 px-4 py-2 text-white">
        Sign in
      </button>
    </div>
  );
}
