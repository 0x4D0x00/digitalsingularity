# Contributing to Silicoid Core

Thank you for your interest in contributing to Silicoid Core — the PotAGI backend! This document outlines the process for contributing to this project.

## Code of Conduct

This project follows a code of conduct to ensure a welcoming environment for all contributors. Please read the full [Code of Conduct](CODE_OF_CONDUCT.md) before contributing. By participating, you agree to:

- Be respectful and inclusive
- Focus on constructive feedback
- Accept responsibility for mistakes
- Show empathy towards other contributors

## How to Contribute

### 1. Reporting Issues

- Use the GitHub issue tracker to report bugs or request features
- Provide detailed steps to reproduce bugs
- Include relevant code snippets, error messages, and environment details
- Check existing issues before creating new ones

### 2. Contributing Code

#### Prerequisites
- Go 1.20+ for backend development
- Understanding of the project architecture (see README.md)
- Familiarity with the codebase structure

#### Development Setup
1. Fork the repository
2. Clone your fork: `git clone https://github.com/YOUR_USERNAME/digitalsingularity.git`
3. Create a feature branch: `git checkout -b feature/your-feature-name`
4. Make your changes
5. Test your changes thoroughly
6. Submit a pull request

#### Code Style and Standards
- Follow Go conventions and best practices
- Use meaningful variable and function names
- Add comments for complex logic
- Write tests for new functionality
- Ensure code passes linting checks

#### Pull Request Process
1. Ensure your PR includes a clear description of the changes
2. Reference any related issues
3. Include tests and documentation updates if applicable
4. Sign the Contributor License Agreement (see CLA.md)
5. Wait for review and address any feedback

### 3. Types of Contributions

#### Code Contributions
- Bug fixes
- New features
- Performance improvements
- Code refactoring

#### Non-Code Contributions
- Documentation improvements
- Tutorial creation
- Issue triage
- Community support

## Development Guidelines

### Project Structure
```
backend/
├── main/           # Main application services
├── silicoid/       # AI model integration layer
├── aibasicplatform/# Basic AI platform services
├── common/         # Shared utilities and security
├── speechsystem/   # Speech processing capabilities
└── modelcontextprotocol/ # MCP integration
```

### Key Areas for Contribution

#### Model Integration
- Add support for new AI model providers
- Improve format conversion between different APIs
- Enhance multimodal file processing

#### Platform Features
- Extend the database-driven prompt management
- Add new tool types and execution modes
- Improve session and context management

#### Infrastructure
- Add new authentication methods
- Improve logging and monitoring
- Enhance deployment configurations

### Testing
- Write unit tests for new functionality
- Update existing tests when modifying code
- Test both individual components and integration scenarios

## Getting Help

- Check the README.md for setup and usage instructions
- Review existing issues and pull requests
- Join community discussions

## Licensing and Intellectual Property

By contributing to this project, you agree that:

1. Your contributions will be licensed under the same terms as the project (Business Source License)
2. You have the right to grant the necessary licenses for your contributions
3. You understand that commercial licensing rights are retained by the project owner (0x4D)

## Recognition

Contributors will be acknowledged in the project documentation and may be listed in future release notes.

## Questions?

If you have questions about contributing, please:
- Check existing documentation
- Open an issue for clarification
- Contact the maintainer: moycox@Outlook.com

Thank you for contributing to Silicoid Core!
