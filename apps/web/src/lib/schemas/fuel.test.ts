import { describe, it, expect } from 'vitest';
import { fuelSchema } from './fuel';

const validWithTotal = {
  date: '2024-06-01T12:00:00Z',
  mileage: 45000,
  gallons: 12.5,
  totalCost: 50.0,
};

const validWithPrice = {
  date: '2024-06-01T12:00:00Z',
  mileage: 45000,
  gallons: 12.5,
  pricePerGallon: 4.0,
};

describe('fuelSchema', () => {
  describe('gallons', () => {
    it('requires gallons', () => {
      const result = fuelSchema.safeParse({
        date: '2024-06-01T12:00:00Z',
        mileage: 45000,
        totalCost: 50.0,
      });
      expect(result.success).toBe(false);
    });

    it('requires gallons to be positive', () => {
      const result = fuelSchema.safeParse({
        ...validWithTotal,
        gallons: 0,
      });
      expect(result.success).toBe(false);
    });

    it('requires gallons to be a positive number', () => {
      const result = fuelSchema.safeParse({
        ...validWithTotal,
        gallons: -1,
      });
      expect(result.success).toBe(false);
    });

    it('accepts valid gallons', () => {
      const result = fuelSchema.safeParse(validWithTotal);
      expect(result.success).toBe(true);
    });
  });

  describe('price or total requirement', () => {
    it('fails when neither totalCost nor pricePerGallon is provided', () => {
      const result = fuelSchema.safeParse({
        date: '2024-06-01T12:00:00Z',
        mileage: 45000,
        gallons: 12.5,
      });
      expect(result.success).toBe(false);
    });

    it('passes when only totalCost is provided', () => {
      const result = fuelSchema.safeParse(validWithTotal);
      expect(result.success).toBe(true);
    });

    it('passes when only pricePerGallon is provided', () => {
      const result = fuelSchema.safeParse(validWithPrice);
      expect(result.success).toBe(true);
    });

    it('passes when both totalCost and pricePerGallon are provided', () => {
      const result = fuelSchema.safeParse({
        ...validWithTotal,
        pricePerGallon: 4.0,
      });
      expect(result.success).toBe(true);
    });

    it('fails when totalCost is 0 and no pricePerGallon', () => {
      const result = fuelSchema.safeParse({
        date: '2024-06-01T12:00:00Z',
        mileage: 45000,
        gallons: 12.5,
        totalCost: 0,
      });
      expect(result.success).toBe(false);
    });

    it('fails when pricePerGallon is 0 and no totalCost', () => {
      const result = fuelSchema.safeParse({
        date: '2024-06-01T12:00:00Z',
        mileage: 45000,
        gallons: 12.5,
        pricePerGallon: 0,
      });
      expect(result.success).toBe(false);
    });
  });

  describe('mileage', () => {
    it('requires mileage', () => {
      const result = fuelSchema.safeParse({
        date: '2024-06-01T12:00:00Z',
        gallons: 12.5,
        totalCost: 50.0,
      });
      expect(result.success).toBe(false);
    });

    it('requires mileage to be non-negative', () => {
      const result = fuelSchema.safeParse({
        ...validWithTotal,
        mileage: -1,
      });
      expect(result.success).toBe(false);
    });
  });
});
