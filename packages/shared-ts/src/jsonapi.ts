export interface JsonApiResource<A, R = Record<string, unknown>> {
  type: string;
  id: string;
  attributes: A;
  relationships?: R;
}

export interface PageMeta {
  total: number;
  totalPages: number;
  number: number;
  size: number;
}

export interface JsonApiDocument<T> {
  data: T;
  meta?: PageMeta;
  links?: Record<string, string>;
}

export interface JsonApiError {
  status: string;
  code: string;
  title: string;
  detail?: string;
  source?: { pointer?: string };
}
