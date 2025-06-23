import { compileFilter, sql } from '../dist/index.js';

console.log('🔍 JSON Filter Examples\n');

// Example 1: Simple equality and comparison
console.log('1. Simple equality and comparison:');
const filter1 = {
  status: 'ACTIVE',
  age: { gte: 18, lt: 65 }
};
const result1 = compileFilter(filter1);
console.log('Filter:', JSON.stringify(filter1, null, 2));
console.log('SQL:', result1.text);
console.log('Values:', result1.values);
console.log();

// Example 2: Set membership and NULL checks
console.log('2. Set membership and NULL checks:');
const filter2 = {
  role: { in: ['ADMIN', 'EDITOR'] },
  deleted_at: { exists: false }
};
const result2 = compileFilter(filter2);
console.log('Filter:', JSON.stringify(filter2, null, 2));
console.log('SQL:', result2.text);
console.log('Values:', result2.values);
console.log();

// Example 3: LIKE and regex patterns
console.log('3. LIKE and regex patterns:');
const filter3 = {
  username: { like: 'john%' },
  email: { regex: '^[A-Za-z0-9._%+-]+@example\\.com$' }
};
const result3 = compileFilter(filter3);
console.log('Filter:', JSON.stringify(filter3, null, 2));
console.log('SQL:', result3.text);
console.log('Values:', result3.values);
console.log();

// Example 4: Logical operators (AND/OR)
console.log('4. Logical operators (AND/OR):');
const filter4 = {
  and: [
    { status: 'ACTIVE' },
    { or: [
        { age: { lt: 18 } },
        { age: { gte: 65 } }
      ]
    },
    { country: { in: ['US', 'CA', 'GB'] } }
  ]
};
const result4 = compileFilter(filter4);
console.log('Filter:', JSON.stringify(filter4, null, 2));
console.log('SQL:', result4.text);
console.log('Values:', result4.values);
console.log();

// Example 5: JSON path querying
console.log('5. JSON path querying:');
const filter5 = {
  'profile.email': { regex: '.*@example\\.org$' },
  'profile.age': { gte: 21 },
  'settings.theme': { in: ['dark', 'light'] }
};
const result5 = compileFilter(filter5);
console.log('Filter:', JSON.stringify(filter5, null, 2));
console.log('SQL:', result5.text);
console.log('Values:', result5.values);
console.log();

// Example 6: Using with the main sql template tag
console.log('6. Integration with sql template tag:');
const whereClause = compileFilter({
  name: { like: 'Acme%' },
  category: { nin: ['ARCHIVED', 'DELETED'] },
  deleted_at: { exists: false }
});

const query = sql`
  SELECT * FROM companies
  WHERE ${sql.raw(whereClause.text)}
`;

console.log('Filter WHERE clause:', whereClause.text);
console.log('Filter values:', whereClause.values);
console.log('Complete query text:', query.text);
console.log('Complete query values:', query.values);
console.log();

console.log('\n=== ADVANCED NESTED LOGICAL OPERATIONS ===\n')

// Complex E-commerce product search with multiple categories and nested conditions
console.log('🛍️  E-commerce Product Search Filter:')
const ecommerceFilter = {
  and: [
    { available: true },
    { price: { gte: 10, lte: 500 } },
    {
      or: [
        // Electronics with brand preferences
        {
          and: [
            { category: 'electronics' },
            {
              or: [
                { brand: { in: ['Apple', 'Samsung', 'Google'] } },
                {
                  and: [
                    { brand: 'Other' },
                    { rating: { gte: 4.5 } },
                    { reviews_count: { gte: 100 } }
                  ]
                }
              ]
            }
          ]
        },
        // Clothing with size and style filters
        {
          and: [
            { category: 'clothing' },
            {
              or: [
                {
                  and: [
                    { 'attributes.gender': 'unisex' },
                    { 'attributes.size': { in: ['S', 'M', 'L'] } }
                  ]
                },
                {
                  and: [
                    { 'attributes.style': { like: '%casual%' } },
                    { discount_percentage: { gte: 20 } }
                  ]
                }
              ]
            }
          ]
        },
        // Books with genre preferences
        {
          and: [
            { category: 'books' },
            {
              or: [
                { genre: { in: ['fiction', 'mystery', 'sci-fi'] } },
                {
                  and: [
                    { genre: 'non-fiction' },
                    { 'metadata.bestseller': true }
                  ]
                }
              ]
            },
            { language: { in: ['english', 'spanish'] } }
          ]
        }
      ]
    }
  ]
}

const ecommerceQuery = compileFilter(ecommerceFilter)
console.log('SQL:', ecommerceQuery.text)
console.log('Values:', ecommerceQuery.values)
console.log('')

// Real-world user permissions system
console.log('🔐 User Permissions & Access Control:')
const permissionsFilter = {
  and: [
    { 'user.active': true },
    {
      or: [
        // Admin users have full access
        { 'user.role': 'ADMIN' },
        // Department managers have access to their department
        {
          and: [
            { 'user.role': 'MANAGER' },
            { 'resource.department': { in: ['engineering', 'design'] } }
          ]
        },
        // Regular users have limited access
        {
          and: [
            { 'user.role': 'USER' },
            {
              or: [
                // Own resources
                { 'resource.owner_id': '${user.id}' },
                // Shared resources
                {
                  and: [
                    { 'resource.shared': true },
                    { 'resource.visibility': { in: ['public', 'team'] } }
                  ]
                },
                // Team resources for team members
                {
                  and: [
                    { 'resource.team_id': { exists: true } },
                    { 'user.team_memberships': { like: '%${resource.team_id}%' } }
                  ]
                }
              ]
            }
          ]
        }
      ]
    }
  ]
}

const permissionsQuery = compileFilter(permissionsFilter)
console.log('SQL:', permissionsQuery.text)
console.log('Values:', permissionsQuery.values)
console.log('')

// Advanced HR employee search with nested criteria
console.log('👥 HR Employee Search (Deep Nesting):')
const hrSearchFilter = {
  and: [
    { status: 'ACTIVE' },
    {
      or: [
        // Senior engineering roles
        {
          and: [
            { department: 'engineering' },
            { level: { gte: 5 } },
            {
              or: [
                { skills: { like: '%javascript%' } },
                { skills: { like: '%python%' } },
                {
                  and: [
                    { 'certifications.aws': true },
                    { experience_years: { gte: 8 } }
                  ]
                }
              ]
            }
          ]
        },
        // Creative design roles
        {
          and: [
            { department: 'design' },
            { level: { gte: 3 } },
            {
              or: [
                { portfolio: { exists: true } },
                {
                  and: [
                    { 'skills.design_tools': { in: ['Figma', 'Sketch', 'Adobe'] } },
                    { 'projects.published': { gte: 10 } }
                  ]
                }
              ]
            }
          ]
        },
        // Leadership positions
        {
          and: [
            { level: { gte: 7 } },
            {
              or: [
                { 'reports.direct_count': { gte: 5 } },
                { 'achievements.leadership_award': true }
              ]
            }
          ]
        }
      ]
    },
    { location: { in: ['SF', 'NYC', 'LA', 'Remote'] } }
  ]
}

const hrQuery = compileFilter(hrSearchFilter)
console.log('SQL:', hrQuery.text)
console.log('Values:', hrQuery.values)
console.log('')

// Multi-tenant SaaS application filter with subscription tiers
console.log('🏢 Multi-tenant SaaS Feature Access:')
const saasAccessFilter = {
  and: [
    { 'tenant.active': true },
    { 'user.status': 'VERIFIED' },
    {
      or: [
        // Enterprise tier gets everything
        { 'subscription.tier': 'ENTERPRISE' },
        // Professional tier with specific features
        {
          and: [
            { 'subscription.tier': 'PROFESSIONAL' },
            { 'subscription.expires_at': { gt: '2024-01-01' } },
            {
              or: [
                { feature_request: 'analytics' },
                { feature_request: 'api_access' },
                {
                  and: [
                    { feature_request: 'advanced_reports' },
                    { 'usage.report_count': { lt: 100 } }
                  ]
                }
              ]
            }
          ]
        },
        // Basic tier with usage limits
        {
          and: [
            { 'subscription.tier': 'BASIC' },
            {
              or: [
                {
                  and: [
                    { feature_request: 'basic_reports' },
                    { 'usage.monthly_reports': { lt: 10 } }
                  ]
                },
                {
                  and: [
                    { feature_request: 'data_export' },
                    { 'usage.monthly_exports': { lt: 5 } }
                  ]
                }
              ]
            }
          ]
        }
      ]
    }
  ]
}

const saasQuery = compileFilter(saasAccessFilter)
console.log('SQL:', saasQuery.text)
console.log('Values:', saasQuery.values)
console.log('')

// Maximum nesting depth example (approaching the 10-level limit)
console.log('🏗️  Maximum Nesting Depth Example (8 levels):')
const maxNestingFilter = {
  and: [                        // Level 1
    { level1: 'value1' },
    {
      or: [                     // Level 2
        { level2a: 'value2a' },
        {
          and: [                // Level 3
            { level3a: 'value3a' },
            {
              or: [             // Level 4
                { level4a: 'value4a' },
                {
                  and: [        // Level 5
                    { level5a: 'value5a' },
                    {
                      or: [     // Level 6
                        { level6a: 'value6a' },
                        {
                          and: [ // Level 7
                            { level7a: 'value7a' },
                            { level8a: 'value8a' } // Level 8
                          ]
                        }
                      ]
                    }
                  ]
                }
              ]
            }
          ]
        }
      ]
    }
  ]
}

const maxNestingQuery = compileFilter(maxNestingFilter)
console.log('SQL:', maxNestingQuery.text)
console.log('Values:', maxNestingQuery.values)
console.log('')

console.log('🎯 Complex filters demonstrate the power of nested logical operations!')
console.log('These patterns allow you to express sophisticated business logic')
console.log('while maintaining type safety and SQL injection protection.\n')

console.log('✅ All examples completed successfully!'); 