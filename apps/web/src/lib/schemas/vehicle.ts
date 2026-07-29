import { z } from 'zod';

const currentYear = new Date().getFullYear();

// Create/edit vehicle form. make/model/year are required (design + plan);
// the rest are optional. Year is bounded to plausible values.
export const vehicleSchema = z.object({
  make: z.string().trim().min(1, 'Make is required'),
  model: z.string().trim().min(1, 'Model is required'),
  year: z
    .number({ invalid_type_error: 'Year is required' })
    .int('Year must be a whole number')
    .min(1900, 'Year looks too old')
    .max(currentYear + 2, 'Year looks too far in the future'),
  nickname: z.string().trim().max(120).optional().or(z.literal('')),
  trim: z.string().trim().max(120).optional().or(z.literal('')),
  vin: z.string().trim().max(32).optional().or(z.literal('')),
  currentMileage: z
    .number({ invalid_type_error: 'Mileage must be a number' })
    .int()
    .min(0, 'Mileage cannot be negative')
    .optional(),
  notes: z.string().trim().max(2000).optional().or(z.literal('')),
});

export type VehicleFormInput = z.infer<typeof vehicleSchema>;
