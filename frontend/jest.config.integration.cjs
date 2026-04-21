/** @type {import('jest').Config} */
module.exports = {
  // Integration tests hit a real network; use Node's native fetch, not jsdom.
  testEnvironment: 'node',
  roots: ['<rootDir>/src'],
  testMatch: ['**/__tests__/integration/**/*.test.{ts,tsx}'],
  moduleFileExtensions: ['ts', 'tsx', 'js', 'jsx', 'json'],
  transform: {
    '^.+\\.tsx?$': [
      'ts-jest',
      {
        tsconfig: '<rootDir>/tsconfig.jest.json',
      },
    ],
  },
  // Integration tests call a live server — allow up to 30 s per test.
  testTimeout: 30000,
  // No DOM setup needed.
  setupFilesAfterEnv: [],
};
