# Roomble

Roomble is a full-stack web application for property discovery and roommate matching. It provides a unified platform for tenants to discover properties and suitable flatmates, while enabling landlords to manage property listings and interact with users.

## Features

### Property Discovery
- Search and filter properties based on user preferences
- View detailed property information
- Bookmark properties for later access
- Review and rate properties
- Landlords can add, edit, list/delist, and delete properties

### Flatmate Discovery
- Discover potential flatmates based on profile information and preferences
- View detailed flatmate profiles
- Bookmark potential flatmates
- Explore other landlord and tenant profiles

### Authentication & Account Management
- Separate authentication workflows for tenants and landlords
- OTP-based verification
- JWT-based authentication
- Forgot-password and password-reset workflows
- Profile creation and profile management

### Real-Time Communication
- Real-time messaging between users using Socket.IO
- Conversation management
- Chat interface for direct communication

### User Management
- Tenant and landlord profiles
- Profile editing and account management
- Secure password handling
- Role-specific application workflows

---

## Tech Stack

### Frontend
- React
- Vite
- JavaScript
- CSS

### Backend
- Node.js
- Express.js
- MongoDB
- Mongoose

### Communication & Authentication
- Socket.IO
- JSON Web Tokens (JWT)
- OTP verification
- Nodemailer

---

## Architecture

Roomble follows a client-server architecture:

```text
                    ┌─────────────────────┐
                    │      React + Vite   │
                    │      Frontend       │
                    └──────────┬──────────┘
                               │
                         HTTP / REST API
                               │
                    ┌──────────▼──────────┐
                    │   Node.js + Express │
                    │      Backend        │
                    └───────┬─────┬───────┘
                            │     │
                 ┌──────────▼─┐ ┌─▼────────────┐
                 │  MongoDB   │ │  Socket.IO   │
                 │  Database  │ │ Real-time    │
                 └────────────┘ │ Messaging    │
                                └───────────────┘

roomble-app/
│
├── backend/
│   ├── controllers/
│   ├── helper_funcs/
│   ├── middlewares/
│   ├── models/
│   ├── routes/
│   ├── Pictures/
│   ├── mongodb.js
│   ├── seed.js
│   └── index.js
│
├── frontend/
│   ├── public/
│   └── src/
│       ├── components/
│       ├── context/
│       ├── css/
│       ├── App.jsx
│       └── main.jsx
│
├── package.json
├── package-lock.json
└── README.md

Getting Started
Prerequisites

Make sure the following are installed:

Node.js
npm
MongoDB
Installation

Clone the repository:

git clone https://github.com/Vaishnavi0090/roomble-app.git
cd roomble-app
Install Backend Dependencies
cd backend
npm install
Install Frontend Dependencies

Open a new terminal:

cd frontend
npm install
Environment Variables

The backend uses environment variables for database access, authentication and email configuration.

Create a .env file inside the backend directory:

MONGODB_URI=your_mongodb_connection_string
SECRET_KEY=your_secret_key
EMAIL_USER=your_email
EMAIL_APP_PASSWORD=your_email_app_password

Do not commit .env files or real credentials to the repository.

Running the Application
Start the Backend

From the backend directory:

node index.js
Start the Frontend

From the frontend directory:

npm run dev

The frontend will be available at the local development URL provided by Vite.

Application Modules

The application is organized around two primary user roles:

Tenant

Tenants can:

Search for properties
Discover potential flatmates
Bookmark properties and flatmates
View and manage their profile
Review properties
Communicate with other users
Landlord

Landlords can:

Create property listings
Update property information
List or delist properties
Delete properties
Manage their profile
Interact with users through the messaging system
Key Backend Components

The backend is organized into modular routes, models, middleware and helper functions.

Models

MongoDB/Mongoose models manage entities such as:

Tenants
Landlords
Properties
Reviews
Conversations
OTP records
Towns
API Routes

Dedicated routes handle:

Authentication
Property management
Property search
Reviews
Bookmarks
User profiles
Password management
Messaging
Development

The frontend is built using reusable React components, while the backend exposes RESTful APIs for application functionality.

The application uses Socket.IO to support real-time communication between users.





