import type { ReactNode } from 'react';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { vehicleSchema, type VehicleFormInput } from '../../../lib/schemas/vehicle';
import { cn } from '../../../lib/utils';

interface VehicleFormProps {
  // 'create' shows all fields; 'edit' only the server-mutable subset
  // (nickname, currentMileage, notes — see PATCH /vehicles/{id}).
  mode: 'create' | 'edit';
  defaultValues?: Partial<VehicleFormInput>;
  onSubmit: (values: VehicleFormInput) => Promise<void> | void;
  onCancel?: () => void;
  submitting?: boolean;
}

function Field({
  label,
  error,
  children,
}: {
  label: string;
  error?: string;
  children: ReactNode;
}) {
  return (
    <div>
      <label className="block text-sm font-medium text-gray-700">{label}</label>
      <div className="mt-1">{children}</div>
      {error && (
        <p className="mt-1 text-sm text-red-600" role="alert">
          {error}
        </p>
      )}
    </div>
  );
}

const inputClass =
  'block w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:border-gray-500 focus:outline-none disabled:bg-gray-50';

export function VehicleForm({
  mode,
  defaultValues,
  onSubmit,
  onCancel,
  submitting,
}: VehicleFormProps) {
  const isCreate = mode === 'create';
  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<VehicleFormInput>({
    resolver: zodResolver(vehicleSchema),
    defaultValues: {
      make: '',
      model: '',
      nickname: '',
      trim: '',
      vin: '',
      notes: '',
      ...defaultValues,
    },
  });

  return (
    <form onSubmit={handleSubmit((values) => onSubmit(values))} className="space-y-4">
      <Field label="Nickname" error={errors.nickname?.message}>
        <input type="text" className={inputClass} {...register('nickname')} />
      </Field>

      {/* make/model/year are immutable after create; show read-only in edit. */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
        <Field label="Make" error={errors.make?.message}>
          <input type="text" className={inputClass} disabled={!isCreate} {...register('make')} />
        </Field>
        <Field label="Model" error={errors.model?.message}>
          <input type="text" className={inputClass} disabled={!isCreate} {...register('model')} />
        </Field>
        <Field label="Year" error={errors.year?.message}>
          <input
            type="number"
            className={inputClass}
            disabled={!isCreate}
            {...register('year', { valueAsNumber: true })}
          />
        </Field>
      </div>

      {isCreate && (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <Field label="Trim" error={errors.trim?.message}>
            <input type="text" className={inputClass} {...register('trim')} />
          </Field>
          <Field label="VIN" error={errors.vin?.message}>
            <input type="text" className={inputClass} {...register('vin')} />
          </Field>
        </div>
      )}

      <Field label="Current mileage" error={errors.currentMileage?.message}>
        <input
          type="number"
          className={inputClass}
          {...register('currentMileage', {
            setValueAs: (v) => (v === '' || v === null || v === undefined ? undefined : Number(v)),
          })}
        />
      </Field>

      <Field label="Notes" error={errors.notes?.message}>
        <textarea rows={3} className={inputClass} {...register('notes')} />
      </Field>

      <div className="flex justify-end gap-2">
        {onCancel && (
          <button
            type="button"
            onClick={onCancel}
            className="rounded-md border border-gray-300 px-4 py-2 text-sm hover:bg-gray-50"
          >
            Cancel
          </button>
        )}
        <button
          type="submit"
          disabled={submitting}
          className={cn(
            'rounded-md bg-gray-900 px-4 py-2 text-sm font-medium text-white hover:bg-gray-800',
            submitting && 'opacity-60',
          )}
        >
          {submitting ? 'Saving…' : isCreate ? 'Add vehicle' : 'Save changes'}
        </button>
      </div>
    </form>
  );
}
