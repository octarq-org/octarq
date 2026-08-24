export type ConfigFieldType = 'string' | 'int' | 'bool' | 'select';

export interface ConfigField {
  name: string;
  label?: string;
  desc?: string;
  type: ConfigFieldType;
  required?: boolean;
  secret?: boolean;
  default?: any;
  enum?: string[];
  min?: number;
  max?: number;
}

export interface ConfigSchema {
  title?: string;
  fields: ConfigField[];
}

export interface JSONSchemaDraft07Property {
  type: 'string' | 'integer' | 'boolean';
  title?: string;
  description?: string;
  default?: any;
  enum?: string[];
  minimum?: number;
  maximum?: number;
  'x-secret'?: boolean;
}

export interface JSONSchemaDraft07 {
  $schema: 'http://json-schema.org/draft-07/schema#';
  type: 'object';
  title?: string;
  properties: Record<string, JSONSchemaDraft07Property>;
  required?: string[];
}
