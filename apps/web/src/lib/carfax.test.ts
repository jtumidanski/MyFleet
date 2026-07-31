import { describe, it, expect } from 'vitest';
import { buildCarfaxUrl, VIN_PLACEHOLDER } from './carfax';

const TEMPLATE = 'https://www.carfax.com/VehicleHistory/p/Report.cfx?vin={vin}';

describe('buildCarfaxUrl', () => {
  it('substitutes the VIN', () => {
    expect(buildCarfaxUrl(TEMPLATE, '1HGCM82633A004352')).toBe(
      'https://www.carfax.com/VehicleHistory/p/Report.cfx?vin=1HGCM82633A004352',
    );
  });

  it('URL-encodes the VIN', () => {
    // A VIN is interpolated into a URL and is never used to build markup, so
    // encoding is the whole defence against a value with reserved characters.
    expect(buildCarfaxUrl(TEMPLATE, 'A B&C=D')).toBe(
      'https://www.carfax.com/VehicleHistory/p/Report.cfx?vin=A%20B%26C%3DD',
    );
  });

  it('trims surrounding whitespace before substituting', () => {
    expect(buildCarfaxUrl(TEMPLATE, '  1HGCM82633A004352  ')).toBe(
      'https://www.carfax.com/VehicleHistory/p/Report.cfx?vin=1HGCM82633A004352',
    );
  });

  it('replaces every occurrence of the placeholder', () => {
    expect(buildCarfaxUrl('https://x.test/{vin}/report?vin={vin}', 'ABC')).toBe(
      'https://x.test/ABC/report?vin=ABC',
    );
  });

  // null means "render no button" — each of these must produce it.
  it('returns null without a usable VIN', () => {
    expect(buildCarfaxUrl(TEMPLATE, undefined)).toBeNull();
    expect(buildCarfaxUrl(TEMPLATE, null)).toBeNull();
    expect(buildCarfaxUrl(TEMPLATE, '')).toBeNull();
    expect(buildCarfaxUrl(TEMPLATE, '   ')).toBeNull();
  });

  it('returns null when the template ignores the VIN', () => {
    // A template without the placeholder would send every user to the same
    // generic page; failing closed is the correct reading of "the button opens
    // THIS vehicle's report".
    expect(buildCarfaxUrl('https://www.carfax.com/', 'ABC')).toBeNull();
  });

  it('returns null for any scheme other than https', () => {
    // The template comes from a runtime ConfigMap, so whoever can edit it
    // chooses what an anchor's href points at. A javascript: template would
    // otherwise be stored XSS.
    expect(buildCarfaxUrl('javascript:alert("{vin}")', 'ABC')).toBeNull();
    expect(buildCarfaxUrl('http://www.carfax.com/?vin={vin}', 'ABC')).toBeNull();
    expect(buildCarfaxUrl('data:text/html,{vin}', 'ABC')).toBeNull();
  });

  it('returns null when the result is not a parseable URL', () => {
    expect(buildCarfaxUrl('not a url {vin}', 'ABC')).toBeNull();
  });

  it('exports the placeholder it recognises', () => {
    expect(VIN_PLACEHOLDER).toBe('{vin}');
  });
});
