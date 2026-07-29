import { z } from 'zod';

export const maintenanceRecordSchema = z.object({
  categoryId: z.string().min(1, 'Category is required'),
  performedAt: z.string().min(1, 'Date performed is required'),
  mileage: z
    .number({ invalid_type_error: 'Mileage must be a number' })
    .int('Mileage must be a whole number')
    .min(0, 'Mileage cannot be negative'),
  cost: z.number({ invalid_type_error: 'Cost must be a number' }).min(0, 'Cost cannot be negative'),
  vendor: z.string().trim().max(200).optional().or(z.literal('')),
  notes: z.string().trim().max(2000).optional().or(z.literal('')),
  documentMediaIds: z.array(z.string()).optional(),
});

export type MaintenanceRecordFormInput = z.infer<typeof maintenanceRecordSchema>;
