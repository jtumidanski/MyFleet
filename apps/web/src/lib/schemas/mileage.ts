import { z } from 'zod';

export const mileageSchema = z.object({
  mileage: z
    .number({ invalid_type_error: 'Mileage must be a number' })
    .int('Mileage must be a whole number')
    .min(1, 'Mileage must be greater than 0'),
  recordedAt: z.string().optional(),
});

export type MileageFormInput = z.infer<typeof mileageSchema>;
