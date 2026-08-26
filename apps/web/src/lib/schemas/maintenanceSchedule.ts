import { z } from 'zod';

const recurrenceTypes = ['time', 'mileage', 'hybrid'] as const;
const scheduleKinds = ['recurring', 'oneTime'] as const;

const intervalMonths = z
  .number({ error: 'Interval months must be a number' })
  .int('Interval months must be a whole number')
  .positive('Interval months must be greater than 0')
  .optional();

const intervalMiles = z
  .number({ error: 'Interval miles must be a number' })
  .int('Interval miles must be a whole number')
  .positive('Interval miles must be greater than 0')
  .optional();

/**
 * Maintenance schedule form schema.
 *
 * Two independent axes, because `recurrenceType` says which axes the schedule
 * is JUDGED ON, not how often it repeats:
 *  - `kind`: 'recurring' | 'oneTime'
 *  - `recurrenceType`: 'time' | 'mileage' | 'hybrid'
 *
 * | kind      | covers time                                  | covers mileage                                   |
 * | recurring | intervalMonths > 0 AND dueDate required      | intervalMiles > 0 AND dueMileage > 0 required    |
 * | oneTime   | dueDate required, intervalMonths forbidden   | dueMileage > 0 required, intervalMiles forbidden |
 *
 * One object with a superRefine rather than a discriminated union: zodResolver
 * over a union fights react-hook-form's single defaultValues object and the
 * shared categoryId / recurrenceType fields, and the form would have to remount
 * on every kind change to keep the resolver honest.
 */
export const maintenanceScheduleSchema = z
  .object({
    categoryId: z.string().min(1, 'Category is required'),
    kind: z.enum(scheduleKinds, { error: 'Schedule kind is required' }),
    recurrenceType: z.enum(recurrenceTypes, { error: 'Recurrence type is required' }),
    intervalMonths,
    intervalMiles,
    /** YYYY-MM-DD from the date input; converted to RFC3339 at the call site. */
    dueDate: z.string().optional(),
    dueMileage: z
      .number({ error: 'Due odometer must be a number' })
      .int('Due odometer must be a whole number')
      .positive('Due odometer must be greater than 0')
      .optional(),
  })
  .superRefine((data, ctx) => {
    const coversTime = data.recurrenceType === 'time' || data.recurrenceType === 'hybrid';
    const coversMileage = data.recurrenceType === 'mileage' || data.recurrenceType === 'hybrid';
    const recurring = data.kind === 'recurring';

    if (coversTime) {
      if (recurring && !data.intervalMonths) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ['intervalMonths'],
          message: 'Interval months is required for this recurrence type',
        });
      }
      if (!data.dueDate) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ['dueDate'],
          message: recurring ? 'First due date is required' : 'Due date is required',
        });
      }
    }

    if (coversMileage) {
      if (recurring && !data.intervalMiles) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ['intervalMiles'],
          message: 'Interval miles is required for this recurrence type',
        });
      }
      if (!data.dueMileage) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ['dueMileage'],
          message: recurring ? 'First due odometer is required' : 'Due odometer is required',
        });
      }
    }

    if (!recurring) {
      if (data.intervalMonths) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ['intervalMonths'],
          message: 'A one-time schedule cannot repeat',
        });
      }
      if (data.intervalMiles) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ['intervalMiles'],
          message: 'A one-time schedule cannot repeat',
        });
      }
    }
  });

export type MaintenanceScheduleFormInput = z.infer<typeof maintenanceScheduleSchema>;

/**
 * Converting a completed one-time schedule into a recurring one. The category
 * is fixed and read-only, and the recurrence runs from the completion point the
 * completion flow already recorded — so there is no category field and no due
 * point to collect, only the recurrence type and its intervals.
 */
export const convertToRecurrenceSchema = z
  .object({
    recurrenceType: z.enum(recurrenceTypes, { error: 'Recurrence type is required' }),
    intervalMonths,
    intervalMiles,
  })
  .superRefine((data, ctx) => {
    const coversTime = data.recurrenceType === 'time' || data.recurrenceType === 'hybrid';
    const coversMileage = data.recurrenceType === 'mileage' || data.recurrenceType === 'hybrid';

    if (coversTime && !data.intervalMonths) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['intervalMonths'],
        message: 'Interval months is required for this recurrence type',
      });
    }
    if (coversMileage && !data.intervalMiles) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['intervalMiles'],
        message: 'Interval miles is required for this recurrence type',
      });
    }
  });

export type ConvertToRecurrenceFormInput = z.infer<typeof convertToRecurrenceSchema>;

/**
 * Marking a schedule complete. The odometer is optional because the reading is
 * auto-filled from the vehicle's latest mileage and the user may clear it.
 */
export const completeScheduleSchema = z.object({
  date: z.string().min(1, 'Date is required'),
  latestMileage: z
    .number({ error: 'Odometer must be a number' })
    .int('Odometer must be a whole number')
    .min(0, 'Odometer cannot be negative')
    .optional(),
});

export type CompleteScheduleFormInput = z.infer<typeof completeScheduleSchema>;
