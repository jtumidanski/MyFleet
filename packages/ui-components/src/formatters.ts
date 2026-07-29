export function formatMoney(n: number): string {
  return new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' }).format(n);
}

export function formatMileage(n: number): string {
  return `${new Intl.NumberFormat('en-US').format(n)} mi`;
}
