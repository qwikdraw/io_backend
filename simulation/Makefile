NAME = server

all: $(NAME)

$(NAME): *.go
	@printf "\e[33;1mBuilding.. \e[0m\n"
	@go build -o $(NAME)
	@printf "\e[32;1mCreated:\e[0m %s\n" $(NAME)

fclean:
	@printf "\e[31;1mFull Cleaning..\e[0m\n"
	@rm -f $(NAME)

re:
	@$(MAKE) fclean 2>/dev/null
	@$(MAKE) 2>/dev/null

.PHONY: fclean all re
