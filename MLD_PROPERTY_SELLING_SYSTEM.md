# 🏢 PROPERTY SELLING SYSTEM - MODEL LOGIC DIAGRAM (MLD)

## 📊 DATABASE RELATIONSHIPS

```
┌─────────────────┐    1:N    ┌─────────────────┐    1:N    ┌─────────────────┐
│     USER        │◄──────────┤  ORGANIZATION   │◄──────────┤   AGENT         │
│                 │           │                 │           │                 │
│ • id (PK)       │           │ • id (PK)       │           │ • id (PK)       │
│ • name          │           │ • name          │           │ • user_id (FK)  │
│ • email         │           │ • owner_id (FK) │           │ • org_id (FK)   │
│ • phone         │           │ • status        │           │ • license_num   │
│ • role          │           │ • license_num   │           │ • status        │
│ • created_at    │           │ • created_at    │           │ • created_at    │
└─────────────────┘           └─────────────────┘           └─────────────────┘
        │                              │                              │
        │                              │                              │
        │ 1:N                          │ 1:N                          │ 1:N
        │                              │                              │
        ▼                              ▼                              ▼
┌─────────────────┐           ┌─────────────────┐           ┌─────────────────┐
│ PROPERTY_TOUR   │           │ PROPERTY_SALE   │           │ PROPERTY_SALE   │
│                 │           │                 │           │                 │
│ • id (PK)       │           │ • id (PK)       │           │ • agent_id (FK) │
│ • property_id   │           │ • org_id (FK)   │           │                 │
│ • customer_id   │           │ • title         │           │                 │
│ • tour_date     │           │ • price         │           │                 │
│ • status        │           │ • status        │           │                 │
│ • created_at    │           │ • is_verified   │           │                 │
└─────────────────┘           │ • created_at    │           │                 │
                              └─────────────────┘           └─────────────────┘
                                      │                              │
                                      │ 1:N                          │ 1:N
                                      │                              │
                                      ▼                              ▼
                              ┌─────────────────┐           ┌─────────────────┐
                              │PROPERTY_INQUIRY │           │ PROPERTY_TOUR   │
                              │                 │           │                 │
                              │ • id (PK)       │           │ • id (PK)       │
                              │ • property_id   │           │ • property_id   │
                              │ • customer_id   │           │ • customer_id   │
                              │ • message       │           │ • tour_date     │
                              │ • status        │           │ • status        │
                              │ • created_at    │           │ • created_at    │
                              └─────────────────┘           └─────────────────┘
```

## 🔄 BUSINESS RULES & CONSTRAINTS

### 1. **ORGANIZATION RULES**
- ✅ Each user can create ONLY ONE organization
- ✅ Organization must be verified before agents can join
- ✅ Organization owner becomes the first agent automatically
- ✅ Organization status: pending → approved → active

### 2. **AGENT RULES**
- ✅ Each user can be an agent for ONLY ONE organization
- ✅ Agent must be approved by organization owner
- ✅ Agent can be assigned to multiple properties
- ✅ Agent status: pending → approved → active

### 3. **PROPERTY RULES**
- ✅ Each organization can create unlimited properties
- ✅ Each property must be verified before publishing
- ✅ Property can be assigned to one agent (optional)
- ✅ Property status: draft → pending_verification → verified → published

### 4. **VERIFICATION WORKFLOW**
```
DRAFT → PENDING_VERIFICATION → VERIFIED → PUBLISHED
  ↓              ↓                ↓           ↓
User creates   Admin reviews   Admin approves  Property goes live
property       property        property        for public viewing
```

### 5. **TOUR BOOKING RULES**
- ✅ Customers can book tours for published properties
- ✅ Tours must be scheduled in advance
- ✅ Agent can confirm/cancel tours
- ✅ Tour status: pending → confirmed → completed

## 📋 ENTITY RELATIONSHIPS

| Entity | Primary Key | Foreign Keys | Relationships |
|--------|-------------|--------------|---------------|
| **USER** | id | - | 1:N with Organization, Agent, PropertyTour, PropertyInquiry |
| **ORGANIZATION** | id | owner_id → User.id | 1:N with Agent, PropertySale |
| **AGENT** | id | user_id → User.id, organization_id → Organization.id | 1:N with PropertySale |
| **PROPERTY_SALE** | id | organization_id → Organization.id, agent_id → Agent.id | 1:N with PropertyTour, PropertyInquiry |
| **PROPERTY_TOUR** | id | property_sale_id → PropertySale.id, customer_id → User.id | - |
| **PROPERTY_INQUIRY** | id | property_sale_id → PropertySale.id, customer_id → User.id | - |

## 🎯 KEY FEATURES

### **For Organizations:**
- Create and manage organization profile
- Add/remove agents
- Create property listings
- Manage property verification status
- View analytics and performance

### **For Agents:**
- Join organization
- Get assigned properties
- Manage property tours
- Respond to inquiries
- Track performance metrics

### **For Customers:**
- Browse verified properties
- Book property tours
- Send inquiries
- View property details and media

### **For Admins:**
- Verify organizations
- Approve property listings
- Manage system-wide settings
- Monitor platform activity

## 🔐 SECURITY & VALIDATION

- ✅ User can only create one organization
- ✅ User can only be agent for one organization
- ✅ Properties must be verified before publishing
- ✅ Tours can only be booked for published properties
- ✅ All financial data is validated and secured
- ✅ Role-based access control for all operations
