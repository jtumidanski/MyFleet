import { z } from 'zod';

/**
 * Fuel log form schema.
 *
 * - gallons: required, must be > 0.
 * - At least one of pricePerGallon or totalCost must be provided (> 0).
 *   Server derives the missing one (design §10.5).
 */
export const fuelSchema = z
  .object({
    date: z.string().min(1, 'Date is required'),
    mileage: z
      .number({ error: 'Mileage must be a number' })
      .int('Mileage must be a whole number')
      .min(0, 'Mileage cannot be negative'),
    gallons: z
      .number({ error: 'Gallons must be a number' })
      .positive('Gallons must be greater than 0'),
    totalCost: z
      .number({ error: 'Total cost must be a number' })
      .positive('Total cost must be greater than 0')
      .optional(),
    pricePerGallon: z
      .number({ error: 'Price per gallon must be a number' })
      .positive('Price per gallon must be greater than 0')
      .optional(),
  })
  .superRefine((data, ctx) => {
    const hasTotal = data.totalCost != null && data.totalCost > 0;
    const hasPrice = data.pricePerGallon != null && data.pricePerGallon > 0;
    if (!hasTotal && !hasPrice) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['pricePerGallon'],
        message: 'Provide price per gallon or total cost (or both)',
      });
    }
  });

export type FuelFormInput = z.infer<typeof fuelSchema>;
