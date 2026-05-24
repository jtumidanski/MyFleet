import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { useNavigate } from 'react-router-dom';
import { toast } from 'sonner';
import { createErrorFromUnknown } from '@myfleet/shared-ts';
import { createFleetSchema, type CreateFleetInput } from '../lib/schemas/fleet';
import { useCreateFleet } from '../lib/hooks/api/fleets';

/**
 * First-run onboarding: create the household fleet. On success the access token
 * is refreshed (so it carries the new activeFleetId) and the user lands on the
 * dashboard.
 */
export function OnboardingPage() {
  const navigate = useNavigate();
  const createFleet = useCreateFleet();
  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<CreateFleetInput>({
    resolver: zodResolver(createFleetSchema),
    defaultValues: { name: '' },
  });

  const onSubmit = handleSubmit(async (values) => {
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
    <div className="flex min-h-screen items-center justify-center bg-gray-50">
      <form
        onSubmit={onSubmit}
        className="w-full max-w-md rounded-lg border border-gray-200 bg-white p-8 shadow-sm"
      >
        <h1 className="text-2xl font-semibold text-gray-900">Set up your fleet</h1>
        <p className="mt-2 text-sm text-gray-500">
          Give your household fleet a name to get started.
        </p>

        <div className="mt-6">
          <label htmlFor="name" className="block text-sm font-medium text-gray-700">
            Fleet name
          </label>
          <input
            id="name"
            type="text"
            autoFocus
            {...register('name')}
            className="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-gray-500 focus:outline-none"
            placeholder="The Smith Household"
          />
          {errors.name && (
            <p className="mt-1 text-sm text-red-600" role="alert">
              {errors.name.message}
            </p>
          )}
        </div>

        <button
          type="submit"
          disabled={isSubmitting}
          className="mt-6 w-full rounded-md bg-gray-900 px-4 py-2 text-sm font-medium text-white hover:bg-gray-800 disabled:opacity-60"
        >
          {isSubmitting ? 'Creating…' : 'Create fleet'}
        </button>
      </form>
    </div>
  );
}
