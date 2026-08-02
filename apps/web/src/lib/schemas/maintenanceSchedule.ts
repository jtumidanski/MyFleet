import { z } from 'zod';

const recurrenceTypes = ['time', 'mileage', 'hybrid'] as const;

/**
 * Maintenance schedule form schema.
 *
 * Recurrence-type conditional validation:
 *  - time:    intervalMonths required (> 0), intervalMiles not required.
 *  - mileage: intervalMiles required (> 0), intervalMonths not required.
 *  - hybrid:  both intervalMonths and intervalMiles required (> 0).
 */
export const maintenanceScheduleSchema = z
  .object({
    categoryId: z.string().min(1, 'Category is required'),
    recurrenceType: z.enum(recurrenceTypes, {
      required_error: 'Recurrence type is required',
    }),
    intervalMonths: z
      .number({ invalid_type_error: 'Interval months must be a number' })
      .int('Interval months must be a whole number')
      .positive('Interval months must be greater than 0')
      .optional(),
    intervalMiles: z
      .number({ invalid_type_error: 'Interval miles must be a number' })
      .int('Interval miles must be a whole number')
      .positive('Interval miles must be greater than 0')
      .optional(),
  })
  .superRefine((data, ctx) => {
    const needsMonths = data.recurrenceType === 'time' || data.recurrenceType === 'hybrid';
    const needsMiles = data.recurrenceType === 'mileage' || data.recurrenceType === 'hybrid';

    if (needsMonths && !data.intervalMonths) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['intervalMonths'],
        message: 'Interval months is required for this recurrence type',
      });
    }
    if (needsMiles && !data.intervalMiles) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['intervalMiles'],
        message: 'Interval miles is required for this recurrence type',
      });
    }
  });

export type MaintenanceScheduleFormInput = z.infer<typeof maintenanceScheduleSchema>;

/**
 * Marking a schedule complete. The odometer is optional because the reading is
 * auto-filled from the vehicle's latest mileage and the user may clear it.
 */
export const completeScheduleSchema = z.object({
  date: z.string().min(1, 'Date is required'),
  latestMileage: z
    .number({ invalid_type_error: 'Odometer must be a number' })
    .int('Odometer must be a whole number')
    .min(0, 'Odometer cannot be negative')
    .optional(),
});

export type CompleteScheduleFormInput = z.infer<typeof completeScheduleSchema>;
